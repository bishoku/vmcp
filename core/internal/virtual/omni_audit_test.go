package virtual

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/correlation"
	"github.com/bishoku/vmcp/core/internal/omni"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
)

func TestOmniAuditBridgeDurabilityPolicyIdentityAndParent(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	ctx := context.Background()
	db := testdb.New(t)
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	pool, err := store.Connect(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	box, _ := secrets.NewBox(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	runtimeCtx, cancel := context.WithCancel(ctx)
	recorder := audit.NewRuntime(runtimeCtx, pool, box, slog.New(slog.NewTextHandler(io.Discard, nil)), "strict")
	defer func() { cancel(); <-recorder.Done() }()
	rig := newRig(t)
	h := rig.handler
	defer h.Sessions.Close()
	h.Audit = recorder
	snap := h.Snap.Snapshot()
	v := snap.VirtualServers["support"]
	v.OmniEnabled = true
	snap.VirtualServers[v.Slug] = v
	snap.AgentKeys = map[string]config.AgentKey{string(store.HashKey("runtime-key")): {AuditCorrelation: correlation.Config{ConversationID: "/client~1conversation", RootConversationID: "/client~1root"}, ID: "runtime-id", Slug: "agent", Servers: map[string]bool{"support": true}}}
	policy := audit.DefaultSettings()
	policy.Mode = "strict"
	policy.Revision = 7
	policy.Scopes["omni"] = audit.Capture{Request: true, Response: true}
	policy.Scopes["omni_diagnostics"] = audit.Capture{Response: true}
	policy.Scopes["virtual"] = audit.Capture{Request: true, Response: true}
	policy.Scopes["upstream"] = audit.Capture{Request: true, Response: true}
	policy.Omni.Candidates = true
	policy.Rules = []audit.Rule{{Name: "customer", Pattern: "CUST-[0-9]+"}}
	snap.Audit = policy
	secret := strings.Repeat("s", 32)
	mux := http.NewServeMux()
	h.RegisterOmni(mux, secret)
	dp := httptest.NewServer(mux)
	defer dp.Close()
	backend, _ := omni.NewBackend(dp.URL, secret)
	index := omni.NewIndex(nil)
	index.Refresh(ctx, backend)
	server := (&omni.Server{Backend: backend, Index: index, Auditor: &omni.GatewayAudit{Backend: backend}}).Handler()
	call := func(name string, args any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 42, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args, "_meta": map[string]any{"client/conversation": "child", "client/root": "root"}}})
		r := httptest.NewRequest("POST", "/mcp/v1/omni", bytes.NewReader(raw))
		r.Header.Set("Authorization", "Bearer runtime-key")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		return w
	}
	out := call("discover_tools", map[string]any{"query": "tables CUST-123"})
	if out.Code != 200 {
		t.Fatalf("discovery %d %s", out.Code, out.Body)
	}
	var root string
	if err := pool.QueryRow(ctx, `SELECT operation_id::text FROM audit_events WHERE scope='omni' AND stage='outcome'`).Scan(&root); err != nil {
		t.Fatal(err)
	}
	events, _, err := (&store.Store{Pool: pool, Box: box}).Reader().ListAudit(ctx, store.AuditFilter{Activity: "omni", OperationID: root}, 100)
	if err != nil || len(events) < 3 {
		t.Fatalf("operation audit missing %v %v", events, err)
	}
	for _, event := range events {
		if event.ConversationID != "child" || event.RootConversationID != "root" || event.ConversationGroupID == "" {
			t.Fatalf("missing root correlation: %+v", event)
		}
		if (event.AgentKey == nil || *event.AgentKey != "agent") || event.ActorID != "runtime-id" || event.PolicyRevision != 7 {
			t.Fatalf("unverified identity/policy: %+v", event)
		}
	}
	var cipher []byte
	var eventID string
	if err := pool.QueryRow(ctx, `SELECT p.event_id::text,p.ciphertext FROM audit_payloads p JOIN audit_events e USING(event_id) WHERE e.scope='omni' AND e.stage='admission'`).Scan(&eventID, &cipher); err != nil {
		t.Fatal(err)
	}
	raw, err := box.OpenBound(cipher, eventID, "audit.payload")
	if err != nil || bytes.Contains(raw, []byte("CUST-123")) || !bytes.Contains(raw, []byte("[REDACTED]")) {
		t.Fatalf("masking: %s %v", raw, err)
	}
	// Four Omni tools, and an explicit invocation parent for normal virtual/upstream audit.
	if out := call("discover_server", map[string]any{"query": "tables"}); out.Code != 200 {
		t.Fatal(out.Body)
	}
	if out := call("get_tool_schema", map[string]any{"tool_ref": omni.Ref("support", "tables")}); out.Code != 200 {
		t.Fatal(out.Body)
	}
	invoked := call("invoke_tool", map[string]any{"tool_ref": omni.Ref("support", "tables"), "arguments": map[string]any{}})
	if invoked.Code != 200 || !strings.Contains(invoked.Body.String(), "pg:list_tables") {
		t.Fatalf("invoke %d %s", invoked.Code, invoked.Body)
	}
	var linked int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events child JOIN audit_events parent ON parent.operation_id=child.parent_operation_id AND parent.stage='admission' WHERE child.scope='virtual' AND child.stage='outcome' AND parent.scope='omni' AND parent.tool='invoke_tool'`).Scan(&linked); err != nil || linked != 1 {
		t.Fatalf("parent link %d %v", linked, err)
	}
	var invokeRoot string
	if err := pool.QueryRow(ctx, `SELECT operation_id::text FROM audit_events WHERE scope='omni' AND tool='invoke_tool' AND stage='outcome'`).Scan(&invokeRoot); err != nil {
		t.Fatal(err)
	}
	descendants, _, err := (&store.Store{Pool: pool}).Reader().ListAudit(ctx, store.AuditFilter{Activity: "omni", OperationID: invokeRoot}, 100)
	if err != nil {
		t.Fatal(err)
	}
	foundUpstream := false
	for _, event := range descendants {
		if event.ConversationID != "child" || event.RootConversationID != "root" || event.ConversationGroupID == "" {
			t.Fatalf("child correlation: %+v", event)
		}
		foundUpstream = foundUpstream || event.Scope == "upstream"
	}
	if !foundUpstream {
		t.Fatal("nested upstream descendants missing")
	}

	// Actual provider requests/responses are separate encrypted scopes, not generic
	// trace details, and the known provider key must never survive an echoed body.
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input     []string `json:"input"`
			Documents []string `json:"documents"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if strings.HasSuffix(r.URL.Path, "embeddings") {
			rows := []map[string]any{}
			for n := range body.Input {
				rows = append(rows, map[string]any{"index": n, "embedding": []float32{1, 0}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": rows, "debug": "provider-secret"})
		} else {
			rows := []map[string]any{}
			for n := range body.Documents {
				rows = append(rows, map[string]any{"index": n, "relevance_score": 0.9})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"results": rows, "debug": "provider-secret"})
		}
	}))
	defer model.Close()
	embedder, _ := omni.NewHTTPEmbedder(model.URL+"/embeddings", "mock", "provider-secret", false)
	reranker, _ := omni.NewHTTPReranker(model.URL+"/rerank", "mock", "provider-secret", false, time.Second)
	index.Embedder = embedder
	index.Reranker = reranker
	index.Refresh(ctx, backend)
	snap.Audit.Scopes["omni_embedding"] = audit.Capture{Request: true, Response: true}
	snap.Audit.Scopes["omni_rerank"] = audit.Capture{Request: true, Response: true}
	snap.Audit.VirtualServers["support"] = []audit.Rule{{Name: "local", Pattern: "tables"}}
	out = call("discover_tools", map[string]any{"query": "tables"})
	if out.Code != 200 {
		t.Fatalf("provider discovery %d %s", out.Code, out.Body)
	}
	rows, err := pool.Query(ctx, `SELECT p.event_id::text,p.ciphertext FROM audit_payloads p JOIN audit_events e USING(event_id) WHERE e.scope IN ('omni_embedding','omni_rerank')`)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	localMask := false
	for rows.Next() {
		var id string
		var encrypted []byte
		if err := rows.Scan(&id, &encrypted); err != nil {
			t.Fatal(err)
		}
		plain, err := box.OpenBound(encrypted, id, "audit.payload")
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(plain, []byte("provider-secret")) {
			t.Fatal("provider key leaked")
		}
		if bytes.Contains(plain, []byte("[REDACTED]")) {
			localMask = true
		}
		count++
	}
	rows.Close()
	if count != 4 || !localMask {
		t.Fatalf("provider payload capture/masking missing: %d", count)
	}
	snap.Audit.Scopes["omni_embedding"] = audit.Capture{}
	snap.Audit.Scopes["omni_rerank"] = audit.Capture{}
	if out := call("discover_tools", map[string]any{"query": "tables"}); out.Code != 200 {
		t.Fatal(out.Body)
	}
	var payloadCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_payloads p JOIN audit_events e USING(event_id) WHERE e.scope IN ('omni_embedding','omni_rerank')`).Scan(&payloadCount); err != nil || payloadCount != 4 {
		t.Fatal("disabled model capture recorded payload", payloadCount, err)
	}
	// Stateless tickets work across replicas, are identity/IP bound, and pin old settings.
	gateway := &omni.GatewayAudit{Backend: backend}
	op, err := gateway.Begin(ctx, "Bearer runtime-key", "127.0.0.1", "", omni.AuditStart{Tool: "discover_tools", Payload: json.RawMessage(`{"query":"CUST-456"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err := gateway.Complete(ctx, "Bearer different", "127.0.0.1", "", omni.AuditFinish{Ticket: op.Ticket, Status: "ok"}); err == nil {
		t.Fatal("ticket stolen across caller")
	}
	snap.Audit = audit.DefaultSettings()
	snap.Audit.Revision = 8
	// Completion after revocation must still retain the original verified identity and policy.
	snap.AgentKeys = map[string]config.AgentKey{}
	if err := gateway.Complete(ctx, "Bearer runtime-key", "127.0.0.1", "", omni.AuditFinish{Ticket: op.Ticket, Status: "denied", Reason: "authorization_changed", Payload: []byte(`{"query":"CUST-456"}`)}); err != nil {
		t.Fatal(err)
	}
	var pinned int64
	if err := pool.QueryRow(ctx, `SELECT policy_revision FROM audit_events WHERE operation_id=$1 AND stage='outcome'`, op.ID).Scan(&pinned); err != nil || pinned != 7 {
		t.Fatal("policy not pinned", pinned, err)
	}
	// Restore auth and inject a storage failure: no underlying operation is dispatched.
	snap.AgentKeys = map[string]config.AgentKey{string(store.HashKey("runtime-key")): {AuditCorrelation: correlation.Config{ConversationID: "/client~1conversation", RootConversationID: "/client~1root"}, ID: "runtime-id", Slug: "agent", Servers: map[string]bool{"support": true}}}
	_, err = pool.Exec(ctx, `CREATE FUNCTION fail_omni_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.scope='omni' THEN RAISE EXCEPTION 'audit unavailable' USING ERRCODE='53100'; END IF;RETURN NEW;END $$;CREATE TRIGGER fail_omni_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_omni_audit()`)
	if err != nil {
		t.Fatal(err)
	}
	if out := call("invoke_tool", map[string]any{"tool_ref": omni.Ref("support", "tables"), "arguments": map[string]any{}}); out.Code != 503 {
		t.Fatal("strict audit failure allowed invoke", out.Code)
	}
}
