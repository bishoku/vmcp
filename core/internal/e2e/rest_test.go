package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/pii"
	"github.com/bishoku/vmcp/core/internal/restapi"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/upstream"
)

func TestRESTAndMCPMixedToolsIdentityPolicyAndRevocation(t *testing.T) {
	e := setup(t)
	var calls atomic.Int32
	var seenID atomic.Value
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		seenID.Store(r.Header.Get("X-VMCP-Agent-Id"))
		if r.Header.Get("X-VMCP-Auth-Type") != "agent-key" || r.Header.Get("X-VMCP-Virtual-Server") != "mixed" {
			t.Error("incorrect trusted metadata")
		}
		_, _ = w.Write([]byte(`{"answer":42,"email":"person@example.com"}`))
	}))
	defer service.Close()
	db := &store.Store{Pool: e.pool, Box: e.syncer.Box}
	cfg := restapi.Config{Auth: restapi.Auth{Type: "none"}, AllowInsecureHTTP: true, TimeoutSeconds: 3, ForwardIdentity: true, RequireIdentity: true, Operations: []restapi.Operation{{Name: "answer", Enabled: true, Method: "GET", Path: "/answer", ReadOnly: true, ResponseSchema: json.RawMessage(`{"type":"object"}`)}}}
	if err := db.Do(e.ctx, func(tx *store.Tx) error {
		if _, _, err := tx.PutUpstream(e.ctx, "rest", store.UpstreamInput{Kind: "rest", REST: &cfg, Name: "REST", URL: service.URL, Transport: "streamable_http", Enabled: true}, nil, true); err != nil {
			return err
		}
		_, _, err := tx.PutVirtualServer(e.ctx, "mixed", store.VirtualServerInput{Name: "Mixed", Enabled: true, AuthRequired: true, PII: pii.Config{Enabled: true, Detectors: []string{"email"}}, Tools: []store.ToolSelection{{Upstream: "rest", Tool: "answer", Alias: "answer"}, {Upstream: "alpha", Tool: "echo", Alias: "echo"}}}, nil, true)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	e.exec(t, `INSERT INTO agent_keys (tenant_id,slug,key_hash,key_prefix) SELECT id,'rest-agent',$1,'vmcp_rest' FROM tenants WHERE slug='default'`, store.HashKey("vmcp_rest_secret"))
	e.exec(t, `INSERT INTO agent_key_servers SELECT k.id,v.id FROM agent_keys k,virtual_servers v WHERE k.slug='rest-agent' AND v.slug='mixed'`)
	var keyID string
	_ = e.pool.QueryRow(e.ctx, `SELECT id::text FROM agent_keys WHERE slug='rest-agent'`).Scan(&keyID)
	wait := func(check func() bool) {
		t.Helper()
		for end := time.Now().Add(5 * time.Second); !check(); time.Sleep(10 * time.Millisecond) {
			if time.Now().After(end) {
				t.Fatal("snapshot did not converge")
			}
		}
	}
	wait(func() bool {
		snap := e.syncer.Snapshot()
		return len(snap.VirtualServers["mixed"].Tools) == 2 && snap.AgentKeys[string(store.HashKey("vmcp_rest_secret"))].Servers["mixed"]
	})
	client := upstream.NewClient(strings.TrimSuffix(e.gwURL, "/agent")+"/mixed", map[string]string{"Authorization": "Bearer vmcp_rest_secret", "X-VMCP-Agent-Id": "spoof"}, nil)
	if _, err := client.Initialize(e.ctx); err != nil {
		t.Fatal(err)
	}
	defer client.Close(context.Background())
	if len(toolNames(t, client)) != 2 {
		t.Fatal("mixed tool catalog missing")
	}
	if result := call(t, client, "echo", `{"text":"hello"}`); result.IsError {
		t.Fatal(result)
	}
	if result := call(t, client, "answer", `{}`); result.IsError || !strings.Contains(result.Content[0].Text, "42") || strings.Contains(result.Content[0].Text, "person@example.com") {
		t.Fatal(result)
	}
	if seenID.Load() != keyID || calls.Load() != 1 {
		t.Fatalf("wrong identity: %v != %s", seenID.Load(), keyID)
	}
	e.exec(t, `UPDATE virtual_servers SET tool_policy='{"default":"allow","tools":{"answer":"deny"}}'::jsonb WHERE slug='mixed'`)
	wait(func() bool { return len(e.syncer.Snapshot().VirtualServers["mixed"].Tools) == 1 })
	if _, err := client.CallTool(e.ctx, "answer", json.RawMessage(`{}`)); err == nil {
		t.Fatal("policy-denied REST tool callable")
	}
	if calls.Load() != 1 {
		t.Fatal("policy-denied request reached service")
	}
	e.exec(t, `UPDATE virtual_servers SET tool_policy='{"default":"allow","tools":{}}'::jsonb WHERE slug='mixed'`)
	wait(func() bool { return len(e.syncer.Snapshot().VirtualServers["mixed"].Tools) == 2 })
	// Removing a REST operation atomically removes it from the effective tool catalog.
	cfg.Operations[0].Enabled = false
	if err := db.Do(e.ctx, func(tx *store.Tx) error {
		_, err := tx.PatchUpstream(e.ctx, "rest", store.UpstreamPatch{REST: &cfg}, nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	wait(func() bool { return len(e.syncer.Snapshot().VirtualServers["mixed"].Tools) == 1 })
	_, err := client.CallTool(e.ctx, "answer", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("disabled operation callable")
	}
	if calls.Load() != 1 {
		t.Fatal("disabled operation reached service")
	}
	e.exec(t, `UPDATE agent_keys SET revoked_at=now() WHERE slug='rest-agent'`)
	wait(func() bool { return len(e.syncer.Snapshot().AgentKeys) == 0 })
	if _, err := client.CallTool(e.ctx, "echo", json.RawMessage(`{"text":"blocked"}`)); err == nil {
		t.Fatal("revoked credential accepted")
	}
}
