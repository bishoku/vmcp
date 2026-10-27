package virtual

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/agentauth"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/testidp"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/omni"
	"github.com/bishoku/vmcp/core/internal/store"
)

func TestOmniEndToEndPublicationInvocationAndRevocation(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	rig := newRig(t)
	h := rig.handler
	defer h.Sessions.Close()
	snap := h.Snap.Snapshot()
	v := snap.VirtualServers["support"]
	v.OmniEnabled = true
	v.Description = "Customer support"
	v.Project = "chat"
	snap.VirtualServers[v.Slug] = v
	key := "runtime-key"
	hash := string(store.HashKey(key))
	snap.AgentKeys = map[string]config.AgentKey{hash: {ID: "key-1", Slug: "agent", Servers: map[string]bool{"support": true}}}
	secret := strings.Repeat("s", 32)
	mux := http.NewServeMux()
	h.RegisterOmni(mux, secret)
	dp := httptest.NewServer(mux)
	defer dp.Close()
	backend, _ := omni.NewBackend(dp.URL, secret)
	index := omni.NewIndex(nil)
	index.Refresh(context.Background(), backend)
	app := &omni.Server{Backend: backend, Index: index}
	gateway := app.Handler()
	call := func(name string, args any) *httptest.ResponseRecorder {
		t.Helper()
		msg, _ := mcp.NewRequest(mcp.NewIntID(42), mcp.MethodToolsCall, map[string]any{"name": name, "arguments": args})
		b, _ := json.Marshal(msg)
		r := httptest.NewRequest("POST", "/mcp/v1/omni", bytes.NewReader(b))
		r.Header.Set("Authorization", "Bearer "+key)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		gateway.ServeHTTP(w, r)
		return w
	}
	search := call("discover_tools", map[string]any{"query": "tables"})
	if search.Code != 200 || !strings.Contains(search.Body.String(), "tables") {
		t.Fatalf("search: %d %s", search.Code, search.Body)
	}
	ref := omni.Ref("support", "tables")
	schema := call("get_tool_schema", map[string]any{"tool_ref": ref})
	if schema.Code != 200 || !strings.Contains(schema.Body.String(), "schema_version") {
		t.Fatalf("schema: %d %s", schema.Code, schema.Body)
	}
	result := call("invoke_tool", map[string]any{"tool_ref": ref, "arguments": map[string]any{}})
	if result.Code != 200 || !strings.Contains(result.Body.String(), "pg:list_tables") {
		t.Fatalf("invoke: %d %s", result.Code, result.Body)
	}
	var response mcp.Message
	_ = json.Unmarshal(result.Body.Bytes(), &response)
	if response.ID.String() != "42" {
		t.Fatal("caller request ID lost")
	}
	stale := call("invoke_tool", map[string]any{"tool_ref": ref, "arguments": map[string]any{}, "schema_version": "old"})
	if stale.Code != 409 {
		t.Fatalf("stale schema %d", stale.Code)
	}
	if os.Getenv("VMCP_REQUIRE_SDK_INTEROP") == "1" {
		server := httptest.NewServer(gateway)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		cmd := exec.CommandContext(ctx, "node", "../../../web/scripts/omni-interop.mjs", server.URL+"/mcp/v1/omni")
		cmd.Env = append(os.Environ(), "VMCP_INTEROP_KEY="+key)
		output, err := cmd.CombinedOutput()
		cancel()
		server.Close()
		if err != nil {
			t.Fatalf("Omni SDK interoperability: %v\n%s", err, output)
		}
	}
	// A removed tool must disappear immediately, without an index refresh.
	snap.Revision++
	v.Tools = v.Tools[:1]
	snap.VirtualServers[v.Slug] = v
	denied := call("get_tool_schema", map[string]any{"tool_ref": ref})
	if denied.Code != 403 && denied.Code != 404 {
		t.Fatalf("removed tool readable: %d", denied.Code)
	}
	search = call("discover_tools", map[string]any{"query": "tables"})
	if strings.Contains(search.Body.String(), `"name":"tables"`) {
		t.Fatal("removed tool remains discoverable")
	}
	snap.Revision++
	v.OmniEnabled = false
	snap.VirtualServers[v.Slug] = v
	denied = call("invoke_tool", map[string]any{"tool_ref": omni.Ref("support", "github__create_issue"), "arguments": map[string]any{}})
	if denied.Code != 403 && denied.Code != 404 {
		t.Fatalf("unpublished server invoked: %d", denied.Code)
	}
	delete(snap.AgentKeys, hash)
	denied = call("discover_tools", map[string]any{"query": "tables"})
	if denied.Code != 401 {
		t.Fatalf("revoked key accepted: %d", denied.Code)
	}
	r := httptest.NewRequest("GET", "/internal/omni/catalog", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatal("private catalog exposed")
	}
}

func TestOmniOAuthIPAndStaleSnapshot(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	rig := newRig(t)
	h := rig.handler
	defer h.Sessions.Close()
	idp := testidp.New(t)
	verifier, err := agentauth.NewVerifier(context.Background(), agentauth.Config{Issuer: idp.URL(), Audience: "vmcp-gateway"})
	if err != nil {
		t.Fatal(err)
	}
	h.JWT = verifier
	snap := h.Snap.Snapshot()
	v := snap.VirtualServers["support"]
	v.OmniEnabled = true
	snap.VirtualServers[v.Slug] = v
	snap.Bindings = []config.Binding{{ID: "binding", Slug: "chat", Conditions: []config.Condition{{Claim: "scope", Op: agentauth.OpContains, Value: "chat"}}, Servers: map[string]bool{"support": true}, Allowed: []netip.Prefix{netip.MustParsePrefix("10.2.0.0/16")}}}
	provider := &checkedSnapshot{}
	provider.snap.Store(snap)
	h.Snap = provider
	h.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	secret := strings.Repeat("s", 32)
	mux := http.NewServeMux()
	h.RegisterOmni(mux, secret)
	request := func(token, ip string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("GET", "/internal/omni/access", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-VMCP-Omni-Secret", secret)
		r.Header.Set("X-VMCP-Client-IP", ip)
		r.Header.Set("X-Forwarded-For", "10.2.1.1")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	token := idp.Mint(t, testidp.Token{Claims: map[string]any{"scope": "chat"}})
	if w := request(token, "10.2.3.4"); w.Code != 200 {
		t.Fatalf("OAuth access %d %s", w.Code, w.Body)
	}
	if w := request(token, "192.0.2.1"); w.Code != 403 {
		t.Fatalf("IP restriction bypassed: %d", w.Code)
	}
	wrong := idp.Mint(t, testidp.Token{Audience: []string{"other-app"}, Claims: map[string]any{"scope": "chat"}})
	if w := request(wrong, "10.2.3.4"); w.Code != 401 {
		t.Fatalf("audience accepted: %d", w.Code)
	}
	h.OmniAllowAnonymous = true
	snap.Bindings[0].Servers = map[string]bool{}
	if w := request(token, "10.2.3.4"); w.Code != 200 {
		t.Fatalf("public OAuth with unassigned binding: %d %s", w.Code, w.Body)
	}
	if len(snap.Bindings[0].Servers) != 0 {
		t.Fatal("public auth mutated binding grants")
	}
	if w := request(token, "192.0.2.1"); w.Code != 403 {
		t.Fatalf("public OAuth bypassed binding IP: %d", w.Code)
	}
	if w := request(wrong, "10.2.3.4"); w.Code != 401 {
		t.Fatalf("public OAuth accepted wrong audience: %d", w.Code)
	}
	provider.stale.Store(true)
	if w := request(token, "10.2.3.4"); w.Code != 503 {
		t.Fatalf("stale authorization accepted: %d", w.Code)
	}
}

func TestOmniDoesNotRevealAnotherAssignedProject(t *testing.T) {
	rig := newRig(t)
	h := rig.handler
	defer h.Sessions.Close()
	snap := h.Snap.Snapshot()
	v := snap.VirtualServers["support"]
	v.OmniEnabled = true
	v.Project = "chat"
	snap.VirtualServers[v.Slug] = v
	hidden := v
	hidden.Slug = "payroll"
	hidden.Name = "Confidential payroll"
	hidden.Project = "finance"
	snap.VirtualServers[hidden.Slug] = hidden
	key := "chat-only-key"
	snap.AgentKeys = map[string]config.AgentKey{string(store.HashKey(key)): {ID: "chat-key", Slug: "chat-agent", Servers: map[string]bool{"support": true}}}
	secret := strings.Repeat("x", 32)
	mux := http.NewServeMux()
	h.RegisterOmni(mux, secret)
	dp := httptest.NewServer(mux)
	defer dp.Close()
	backend, _ := omni.NewBackend(dp.URL, secret)
	index := omni.NewIndex(nil)
	index.Refresh(context.Background(), backend)
	app := (&omni.Server{Backend: backend, Index: index}).Handler()
	call := func(name string, args any) *httptest.ResponseRecorder {
		t.Helper()
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
		r := httptest.NewRequest("POST", "/mcp/v1/omni", bytes.NewReader(b))
		r.Header.Set("Authorization", "Bearer "+key)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		return w
	}
	for _, name := range []string{"discover_server", "discover_tools"} {
		w := call(name, map[string]any{"query": "tables", "project": "finance"})
		if w.Code != 200 || strings.Contains(w.Body.String(), "payroll") || strings.Contains(w.Body.String(), "Confidential") {
			t.Fatalf("hidden project discovered: %d %s", w.Code, w.Body)
		}
	}
	for _, name := range []string{"get_tool_schema", "invoke_tool"} {
		w := call(name, map[string]any{"tool_ref": omni.Ref("payroll", "tables"), "arguments": map[string]any{}})
		if w.Code != 403 && w.Code != 404 {
			t.Fatalf("hidden schema/invocation allowed: %s %d", name, w.Code)
		}
	}
}

type omniAuditCapture struct {
	mu     sync.Mutex
	events []audit.Event
}

func (c *omniAuditCapture) Record(e audit.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

func TestOmniAnonymousPublicationAndFreshAuthorization(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	rig := newRig(t)
	h := rig.handler
	defer h.Sessions.Close()
	snap := h.Snap.Snapshot()
	capture := &omniAuditCapture{}
	h.Audit = capture
	public := snap.VirtualServers["support"]
	public.OmniEnabled = true
	public.AuthRequired = false
	snap.VirtualServers[public.Slug] = public
	private := public
	private.Slug = "private"
	private.AuthRequired = true
	snap.VirtualServers[private.Slug] = private
	hidden := public
	hidden.Slug = "hidden"
	hidden.AuthRequired = true
	snap.VirtualServers[hidden.Slug] = hidden
	unpublished := public
	unpublished.Slug = "unpublished"
	unpublished.OmniEnabled = false
	snap.VirtualServers[unpublished.Slug] = unpublished
	expired := time.Now().Add(-time.Hour)
	snap.AgentKeys = map[string]config.AgentKey{
		string(store.HashKey("valid")):    {ID: "valid", Slug: "agent", Servers: map[string]bool{"private": true}},
		string(store.HashKey("empty")):    {ID: "empty", Slug: "unassigned", Servers: map[string]bool{}},
		string(store.HashKey("expired")):  {ID: "expired", Servers: map[string]bool{"private": true}, ExpiresAt: &expired},
		string(store.HashKey("iplocked")): {ID: "iplocked", Servers: map[string]bool{"private": true}, Allowed: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}},
	}
	secret := strings.Repeat("x", 32)
	mux := http.NewServeMux()
	h.RegisterOmni(mux, secret)
	dp := httptest.NewServer(mux)
	defer dp.Close()
	backend, _ := omni.NewBackend(dp.URL, secret)
	index := omni.NewIndex(nil)
	index.Refresh(context.Background(), backend)
	app := &omni.Server{Backend: backend, Index: index}
	gateway := app.Handler()
	call := func(token, name string, args any) *httptest.ResponseRecorder {
		t.Helper()
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
		r := httptest.NewRequest("POST", "/mcp/v1/omni", bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
		r.RemoteAddr = "192.0.2.1:1000"
		if token != "" {
			r.Header.Set("Authorization", token)
		}
		w := httptest.NewRecorder()
		gateway.ServeHTTP(w, r)
		return w
	}
	query := map[string]any{"query": "tables"}
	if w := call("", "discover_tools", query); w.Code != 401 {
		t.Fatalf("default allows anonymous: %d", w.Code)
	}
	app.AllowAnonymous = true
	if w := call("", "discover_tools", query); w.Code != 401 {
		t.Fatalf("DP opt-in bypassed: %d", w.Code)
	}
	h.OmniAllowAnonymous = true
	for _, name := range []string{"discover_server", "discover_tools"} {
		w := call("", name, query)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "support") || strings.Contains(w.Body.String(), "private") || strings.Contains(w.Body.String(), "hidden") || strings.Contains(w.Body.String(), "unpublished") {
			t.Fatalf("public discovery: %d %s", w.Code, w.Body)
		}
		w = call("Bearer valid", name, query)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "support") || !strings.Contains(w.Body.String(), "private") || strings.Contains(w.Body.String(), "hidden") {
			t.Fatalf("union discovery: %d %s", w.Code, w.Body)
		}
	}
	for _, token := range []string{"Bearer invalid", "Bearer expired", "Basic bad", "Bearer "} {
		if w := call(token, "discover_tools", query); w.Code != 401 {
			t.Fatalf("invalid credential downgraded: %q %d", token, w.Code)
		}
	}
	if w := call("Bearer iplocked", "discover_tools", query); w.Code != 403 {
		t.Fatalf("IP restrictions bypassed: %d", w.Code)
	}
	if w := call("Bearer empty", "discover_tools", query); w.Code != 200 || !strings.Contains(w.Body.String(), "support") || strings.Contains(w.Body.String(), "private") {
		t.Fatalf("unassigned valid credential public access: %d %s", w.Code, w.Body)
	}
	for _, name := range []string{"get_tool_schema", "invoke_tool"} {
		args := map[string]any{"tool_ref": omni.Ref("support", "tables"), "arguments": map[string]any{}}
		w := call("", name, args)
		if w.Code != 200 {
			t.Fatalf("public %s: %d %s", name, w.Code, w.Body)
		}
		if name == "invoke_tool" && !strings.Contains(w.Body.String(), "pg:list_tables") {
			t.Fatal(w.Body)
		}
		for _, server := range []string{"private", "hidden", "unpublished"} {
			args["tool_ref"] = omni.Ref(server, "tables")
			w = call("", name, args)
			if w.Code != 401 && w.Code != 403 && w.Code != 404 {
				t.Fatalf("protected %s %s: %d", server, name, w.Code)
			}
		}
	}
	capture.mu.Lock()
	audited := false
	for _, e := range capture.events {
		if e.Action == "omni/tools/call" && e.ActorType == "anonymous" && e.ClientIP == "192.0.2.1" && e.VirtualServer == "support" && e.Tool == "tables" && e.Status == "ok" {
			audited = true
		}
	}
	capture.mu.Unlock()
	if !audited {
		t.Fatal("anonymous invocation audit missing identity/IP/target")
	}
	// Publication/auth changes are effective even while the search index is stale.
	public.AuthRequired = true
	snap.VirtualServers[public.Slug] = public
	snap.Revision++
	if w := call("", "discover_tools", query); w.Code != 403 {
		t.Fatalf("anonymous access survives auth change: %d %s", w.Code, w.Body)
	}
	public.AuthRequired = false
	public.OmniEnabled = false
	snap.VirtualServers[public.Slug] = public
	snap.Revision++
	if w := call("", "discover_tools", query); w.Code != 403 {
		t.Fatalf("anonymous access survives unpublish: %d", w.Code)
	}
}
