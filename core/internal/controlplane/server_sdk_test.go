//go:build sdkinterop

package controlplane_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/omni"
	"github.com/bishoku/vmcp/core/internal/snapshot"
	"github.com/bishoku/vmcp/core/internal/virtual"
)

type sdkProcess struct{ url, executions string }

func (p sdkProcess) count(t *testing.T, value string) int {
	t.Helper()
	b, err := os.ReadFile(p.executions)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	n := 0
	for _, v := range strings.Fields(string(b)) {
		if v == value || value == "" {
			n++
		}
	}
	return n
}

func startSDK(t *testing.T, sdk, profile string) sdkProcess {
	t.Helper()
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(t.TempDir(), "url")
	var argv []string
	switch sdk {
	case "go":
		argv = []string{os.Getenv("VMCP_GO_SDK_SERVER")}
	case "typescript":
		argv = []string{"node", filepath.Join(root, "web/scripts/upstream-sdk-server.mjs")}
	case "fastmcp":
		project := "python"
		if profile == "legacy" {
			project = "python-legacy"
		}
		argv = []string{filepath.Join(root, "tests/sdk-servers", project, ".venv/bin/python"), filepath.Join(root, "tests/sdk-servers/python/server.py")}
	}
	argv = append(argv, profile, ready)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	executions := filepath.Join(t.TempDir(), "executions")
	cmd.Env = append(os.Environ(), "VMCP_EXECUTIONS="+executions)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	deadline := time.After(30 * time.Second)
	for {
		if b, err := os.ReadFile(ready); err == nil && len(b) > 0 {
			return sdkProcess{string(b), executions}
		}
		select {
		case <-done:
			t.Fatal("SDK server exited before startup")
		case <-deadline:
			t.Fatal("SDK startup timeout")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

type wireEvent struct{ method, version, routing, name, sid, value, metadataVersion string }
type observedSDK struct {
	mu        sync.Mutex
	events    []wireEvent
	fault     atomic.Int32
	target    atomic.Pointer[url.URL]
	dropReply atomic.Bool
}

func (o *observedSDK) take() []wireEvent {
	o.mu.Lock()
	defer o.mu.Unlock()
	events := o.events
	o.events = nil
	return events
}
func (o *observedSDK) serve(t *testing.T, target string) *httptest.Server {
	u, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	o.target.Store(u)
	proxy := &httputil.ReverseProxy{ModifyResponse: func(r *http.Response) error {
		if r.Request.Header.Get("Mcp-Method") == mcp.MethodToolsCall && o.dropReply.CompareAndSwap(true, false) {
			return fmt.Errorf("injected lost reply after dispatch")
		}
		return nil
	}, Rewrite: func(p *httputil.ProxyRequest) {
		u := o.target.Load()
		p.Out.URL.Scheme = u.Scheme
		p.Out.URL.Host = u.Host
		p.Out.URL.Path = u.Path
		p.Out.Host = u.Host
	}}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read", 400)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var rpc struct {
			Method string
			Params struct {
				Name      string
				Meta      map[string]any `json:"_meta"`
				Arguments struct{ Value string }
			}
		}
		_ = json.Unmarshal(raw, &rpc)
		method := rpc.Method
		if r.Method == "DELETE" {
			method = "DELETE"
		}
		e := wireEvent{method: method, version: r.Header.Get("MCP-Protocol-Version"), routing: r.Header.Get("Mcp-Method"), name: r.Header.Get("Mcp-Name"), sid: r.Header.Get("Mcp-Session-Id"), value: rpc.Params.Arguments.Value}
		e.metadataVersion, _ = rpc.Params.Meta["io.modelcontextprotocol/protocolVersion"].(string)
		if rpc.Method == "tools/call" && rpc.Params.Name != "echo" {
			t.Errorf("wrong tool %q", rpc.Params.Name)
		}
		o.mu.Lock()
		o.events = append(o.events, e)
		o.mu.Unlock()
		if code := o.fault.Load(); code != 0 {
			if code == -1 {
				<-r.Context().Done()
				return
			}
			http.Error(w, "injected failure", int(code))
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

func TestRealUpstreamServers(t *testing.T) {
	t.Setenv("VMCP_MCP_20260728", "1")
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	for _, sdk := range []string{"go", "typescript", "fastmcp"} {
		for _, profile := range []string{"legacy", "modern", "dual"} {
			t.Run(sdk+"/"+profile, func(t *testing.T) { checkSDKProfile(t, sdk, profile) })
		}
	}
}

func checkSDKProfile(t *testing.T, sdk, profile string) {
	a := setup(t)
	target := startSDK(t, sdk, profile)
	ctxProbe, cancelProbe := context.WithTimeout(context.Background(), 15*time.Second)
	output, err := exec.CommandContext(ctxProbe, "node", "../../../web/scripts/upstream-sdk-profile.mjs", target.url, profile).CombinedOutput()
	cancelProbe()
	if err != nil {
		t.Fatalf("direct profile validation: %v %s", err, output)
	}
	observer := new(observedSDK)
	s := observer.serve(t, target.url)
	expected := mcp.ProtocolVersion20260728
	if profile == "legacy" {
		expected = mcp.ProtocolVersion20250618
	}
	config := map[string]any{"url": s.URL, "transport": "streamable_http"}
	_, connection := a.expect(200, "POST", "/api/v1/connection-tests", config)
	if connection["ok"] != true || connection["protocolVersion"] != expected {
		t.Fatalf("connection test: %v", connection)
	}
	t.Logf("control-plane revision=%s", connection["protocolVersion"])
	checkSchema := func(body any) {
		t.Helper()
		var tools []struct {
			Name        string
			InputSchema struct {
				Properties map[string]struct{ Type string }
				Required   []string
			}
		}
		raw, err := json.Marshal(body)
		if err != nil || json.Unmarshal(raw, &tools) != nil || len(tools) != 1 || tools[0].Name != "echo" || tools[0].InputSchema.Properties["value"].Type != "string" || !reflect.DeepEqual(tools[0].InputSchema.Required, []string{"value"}) {
			t.Fatalf("unexpected echo schema: %s", raw)
		}
	}
	if target.count(t, "") != 0 {
		t.Fatal("discovery executed a callback")
	}
	checkSchema(connection["tools"])
	info, _ := connection["serverInfo"].(map[string]any)
	if name, _ := info["name"].(string); name != "" && name != "interop-"+sdk {
		t.Fatalf("unexpected server identity %v", info)
	}
	config["slug"] = "fixture"
	config["name"] = "Fixture"
	a.expect(201, "POST", "/api/v1/upstreams", config)
	a.expect(200, "POST", "/api/v1/upstreams/fixture/refresh-tools", nil)
	var discovers, inits, lists int
	for _, e := range observer.take() {
		switch e.method {
		case "server/discover":
			discovers++
		case "initialize":
			inits++
		case "tools/list":
			lists++
			if e.version != expected {
				t.Errorf("control-plane list revision %q", e.version)
			}
		}
		if e.method == "tools/call" {
			t.Fatal("discovery executed a tool")
		}
	}
	if discovers != 2 || lists != 2 || (profile == "legacy" && inits != 2) || (profile != "legacy" && inits != 0) {
		t.Fatalf("control-plane discovery wire: discover=%d initialize=%d list=%d", discovers, inits, lists)
	}
	a.expect(201, "POST", "/api/v1/virtual-servers", map[string]any{"slug": "interop", "name": "Interop", "omniEnabled": true, "tools": []map[string]string{{"upstream": "fixture", "tool": "echo", "alias": "say"}}})
	_, auth := a.expect(201, "POST", "/api/v1/agent-keys", map[string]any{"slug": "interop", "virtualServers": []string{"interop"}})
	_, catalog := a.expect(200, "GET", "/api/v1/upstreams/fixture/tools", nil)
	faults := []int32{401, 503}
	if sdk == "go" && profile == "dual" {
		faults = append(faults, -1)
	}
	for _, code := range faults {
		observer.fault.Store(code)
		_, failed := a.expect(200, "POST", "/api/v1/upstreams/fixture/connection-test", nil)
		if failed["ok"] != false {
			t.Fatal("expected failed probe")
		}
		a.expect(502, "POST", "/api/v1/upstreams/fixture/refresh-tools", nil)
		_, retained := a.expect(200, "GET", "/api/v1/upstreams/fixture/tools", nil)
		if !reflect.DeepEqual(catalog["items"], retained["items"]) {
			t.Fatal("failed refresh changed stored tools")
		}
		events := observer.take()
		if len(events) != 2 {
			t.Fatalf("expected two failed discovery requests, got %d", len(events))
		}
		for _, e := range events {
			if e.method != "server/discover" {
				t.Fatalf("HTTP %d caused %s", code, e.method)
			}
		}
	}
	observer.fault.Store(0)
	dp, om := startSDKReplica(t, a)
	ctx := context.Background()
	for _, endpoint := range []string{"virtual", "omni"} {
		for _, client := range []string{"legacy", "modern"} {
			t.Run(endpoint+"/"+client, func(t *testing.T) {
				url := dp.URL + "/mcp/v1/virtual/interop"
				if endpoint == "omni" {
					url = om.URL + "/mcp/v1/omni"
				}
				value := fmt.Sprintf("%s-%s-%s-%s", sdk, profile, endpoint, client)
				ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, "node", "../../../web/scripts/upstream-sdk-client.mjs", url, client, endpoint, value)
				cmd.Env = append(os.Environ(), "VMCP_INTEROP_KEY="+auth["key"].(string))
				output, err := cmd.CombinedOutput()
				events := observer.take()
				executions := target.count(t, value)
				for _, e := range events {
					t.Logf("wire method=%s version=%s route=%s name=%s session=%t", e.method, e.version, e.routing, e.name, e.sid != "")
				}
				t.Logf("callbacks=%d", executions)
				if err != nil {
					t.Fatalf("SDK client: %v %s", err, output)
				}
				calls, probes := 0, 0
				var sessions []string
				var initialized, acknowledged int
				deleted := map[string]bool{}
				for _, e := range events {
					switch e.method {
					case "DELETE":
						deleted[e.sid] = true
					case "initialize":
						initialized++
					case "notifications/initialized":
						acknowledged++
					}
					if expected == mcp.ProtocolVersion20260728 && (e.sid != "" || e.routing != e.method || e.metadataVersion != expected) {
						t.Fatalf("modern metadata or headers for %s", e.method)
					}
					if e.method == mcp.MethodDiscover {
						probes++
					}
					if e.method != "tools/call" {
						continue
					}
					calls++
					sessions = append(sessions, e.sid)
					if e.version != expected || e.value != value || (expected == mcp.ProtocolVersion20260728 && (e.routing != e.method || e.name != "echo" || e.sid != "")) {
						t.Fatalf("unexpected upstream call version=%s route=%s name=%s", e.version, e.routing, e.name)
					}
				}
				if expected == mcp.ProtocolVersion20250618 && (initialized == 0 || initialized != acknowledged) {
					t.Fatal("legacy handshake missing or incomplete")
				}
				if expected == mcp.ProtocolVersion20260728 && initialized != 0 {
					t.Fatal("unexpected legacy handshake")
				}
				for _, sid := range sessions {
					if sid != "" && !deleted[sid] {
						t.Fatal("issued upstream session was not terminated")
					}
				}
				if probes > 1 {
					t.Fatal("warm invocation repeated discovery")
				}
				if calls == 2 && sessions[0] != "" {
					persistent := endpoint == "virtual" && client == "legacy"
					if (sessions[0] == sessions[1]) != persistent {
						t.Fatal("unexpected upstream session reuse")
					}
				}
				if target.count(t, value) != 2 {
					t.Fatal("SDK callback execution count differs from wire count")
				}
				if calls != 2 {
					t.Fatalf("expected two invocations, got %d", calls)
				}
			})
		}
	}
}

func startSDKReplica(t *testing.T, a *api) (*httptest.Server, *httptest.Server) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	syncer := &snapshot.Syncer{Pool: a.pool, Box: a.control.Store.Box, Log: a.control.Log}
	if err := syncer.Start(ctx); err != nil {
		t.Fatal(err)
	}
	manager := virtual.NewManager()
	manager.Store = virtual.PGSessionStore{Pool: a.pool}
	t.Cleanup(manager.Close)
	h := &virtual.Handler{Sessions: manager, Snap: syncer, Log: a.control.Log}
	mux := http.NewServeMux()
	h.Register(mux)
	secret := strings.Repeat("s", 32)
	h.RegisterOmni(mux, secret)
	dp := httptest.NewServer(mux)
	t.Cleanup(dp.Close)
	backend, err := omni.NewBackend(dp.URL, secret)
	if err != nil {
		t.Fatal(err)
	}
	index := omni.NewIndex(nil)
	index.Refresh(ctx, backend)
	om := httptest.NewServer((&omni.Server{Index: index, Backend: backend}).Handler())
	t.Cleanup(om.Close)
	return dp, om
}

// Exercise the production preference lifetime on two independent managers.
// "upgrade" keeps its URL while its server is replaced. "outage" stays modern
// and has a legacy alternative, so both expiry scenarios share the same wait.
func TestRealUpstreamTransitions(t *testing.T) {
	t.Setenv("VMCP_MCP_20260728", "1")
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	a := setup(t)
	old := startSDK(t, "go", "legacy")
	dual := startSDK(t, "typescript", "dual")
	fallback := startSDK(t, "fastmcp", "legacy")
	upgradeWire, outageWire, backupWire := new(observedSDK), new(observedSDK), new(observedSDK)
	upgrade := upgradeWire.serve(t, old.url)
	outage := outageWire.serve(t, dual.url)
	backup := backupWire.serve(t, fallback.url)
	for _, entry := range []struct {
		slug, url    string
		alternatives []string
	}{{"upgrade", upgrade.URL, nil}, {"outage", outage.URL, []string{backup.URL}}} {
		a.expect(201, "POST", "/api/v1/upstreams", map[string]any{"slug": entry.slug, "name": entry.slug, "url": entry.url, "transport": "streamable_http", "fallbackUrls": entry.alternatives})
		a.expect(200, "POST", "/api/v1/upstreams/"+entry.slug+"/refresh-tools", nil)
	}
	a.expect(201, "POST", "/api/v1/virtual-servers", map[string]any{"slug": "interop", "name": "Interop", "omniEnabled": true, "tools": []map[string]string{{"upstream": "upgrade", "tool": "echo", "alias": "say"}, {"upstream": "outage", "tool": "echo", "alias": "write"}}})
	_, key := a.expect(201, "POST", "/api/v1/agent-keys", map[string]any{"slug": "interop", "virtualServers": []string{"interop"}})
	dp, _ := startSDKReplica(t, a)
	_, om := startSDKReplica(t, a)
	token := key["key"].(string)
	virtualURL := dp.URL + "/mcp/v1/virtual/interop"
	omniURL := om.URL + "/mcp/v1/omni"
	post := func(target, sid, method string, params map[string]any, modern, notification bool) (http.Header, *mcp.Message) {
		t.Helper()
		id := mcp.NewIntID(42)
		if notification {
			id = mcp.ID{}
		}
		if modern {
			params["_meta"] = map[string]any{"io.modelcontextprotocol/protocolVersion": mcp.ProtocolVersion20260728, "io.modelcontextprotocol/clientCapabilities": map[string]any{}}
		}
		msg, err := mcp.NewRequest(id, method, params)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(msg)
		req, err := http.NewRequest("POST", target, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+token)
		if sid != "" {
			req.Header.Set("Mcp-Session-Id", sid)
			req.Header.Set("Mcp-Protocol-Version", mcp.ProtocolVersion20250618)
		}
		if modern {
			req.Header.Set("Mcp-Protocol-Version", mcp.ProtocolVersion20260728)
			req.Header.Set("Mcp-Method", method)
			if name, ok := params["name"].(string); ok {
				req.Header.Set("Mcp-Name", name)
			}
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			t.Fatalf("gateway HTTP %s: %s", res.Status, raw)
		}
		if notification {
			return res.Header, nil
		}
		reply, err := mcp.Decode(raw)
		if err != nil {
			t.Fatal(err)
		}
		return res.Header, reply
	}
	h, reply := post(virtualURL, "", "initialize", map[string]any{"clientInfo": map[string]string{"name": "upgrade", "version": "1"}, "protocolVersion": mcp.ProtocolVersion20250618, "capabilities": map[string]any{}}, false, false)
	sid := h.Get("Mcp-Session-Id")
	if sid == "" || reply.Error != nil {
		t.Fatal("legacy virtual session not established")
	}
	post(virtualURL, sid, "notifications/initialized", map[string]any{}, false, true)
	invoke := func(omni bool, name, value string, wire *observedSDK, version string, fail bool) {
		t.Helper()
		wire.take()
		params := map[string]any{"name": name, "arguments": map[string]string{"value": value}}
		target, session := virtualURL, sid
		if omni {
			target, session = omniURL, ""
			params = map[string]any{"name": "invoke_tool", "arguments": map[string]any{"tool_ref": "interop/" + name, "arguments": params["arguments"]}}
		}
		_, m := post(target, session, "tools/call", params, omni, false)
		if (m.Error != nil) != fail || (!fail && !strings.Contains(string(m.Result), value)) {
			t.Fatalf("invocation %s failed: %+v", value, m)
		}
		var n int
		for _, e := range wire.take() {
			if e.method == "tools/call" {
				n++
				if e.version != version || e.value != value {
					t.Fatalf("wire: %+v", e)
				}
			}
		}
		if n != 1 {
			t.Fatalf("tool transmissions %d", n)
		}
	}
	legacy, modern := mcp.ProtocolVersion20250618, mcp.ProtocolVersion20260728
	invoke(false, "say", "a-before", upgradeWire, legacy, false)
	invoke(true, "say", "b-before", upgradeWire, legacy, false)
	invoke(false, "write", "warm-primary", outageWire, modern, false)
	warmed := time.Now()
	upgradedURL, err := url.Parse(dual.url)
	if err != nil {
		t.Fatal(err)
	}
	upgradeWire.target.Store(upgradedURL)
	_, result := a.expect(200, "POST", "/api/v1/connection-tests", map[string]any{"url": upgrade.URL, "transport": "streamable_http"})
	if result["protocolVersion"] != modern {
		t.Fatalf("fresh control-plane revision: %v", result)
	}
	invoke(true, "say", "b-cached", upgradeWire, legacy, false)
	timer := time.NewTimer(time.Until(warmed.Add(5*time.Minute + time.Second)))
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-t.Context().Done():
		t.Fatal("test canceled before preference expiry")
	}
	invoke(false, "say", "a-upgraded", upgradeWire, modern, false)
	invoke(true, "say", "b-upgraded", upgradeWire, modern, false)
	outageWire.take()
	outageWire.fault.Store(503)
	invoke(false, "write", "fallback-once", backupWire, legacy, false)
	for _, e := range outageWire.take() {
		if e.method != "server/discover" {
			t.Fatalf("unexpected dispatch to unavailable primary: %v", e)
		}
	}
	if fallback.count(t, "fallback-once") != 1 || dual.count(t, "fallback-once") != 0 {
		t.Fatal("failover execution count")
	}
	outageWire.fault.Store(0)
	outageWire.dropReply.Store(true)
	invoke(true, "write", "ambiguous-write", outageWire, modern, true)
	if dual.count(t, "ambiguous-write") != 1 || fallback.count(t, "ambiguous-write") != 0 {
		t.Fatal("ambiguous write callback count")
	}
}
