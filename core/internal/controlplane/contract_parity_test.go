package controlplane_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestP07ContractsAndConcurrency(t *testing.T) {
	a := setup(t)
	examples := map[string]any{}
	create := map[string]any{"slug": "fixture", "name": "Fixture", "url": a.fixture.URL, "headers": map[string]string{"Authorization": "Bearer up-secret"}}
	status, _, up := a.do("POST", "/api/v1/upstreams", create, nil)
	if status != 201 {
		t.Fatalf("create %d %+v", status, up)
	}
	examples["UpstreamCreate"] = create
	examples["Upstream"] = up
	status, _, result := a.do("POST", "/api/v1/upstreams/fixture/connection-test", nil, nil)
	if status != 200 || result["ok"] != true {
		t.Fatalf("stored test: %d %+v", status, result)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "up-secret") || strings.Contains(string(encoded), "Authorization") {
		t.Fatal("stored credentials leaked")
	}
	if status, _, _ := a.do("POST", "/api/v1/upstreams/fixture/connection-test", nil, map[string]string{"Authorization": ""}); status != 401 {
		t.Fatalf("unauthenticated stored test: %d", status)
	}
	a.do("POST", "/api/v1/upstreams/fixture/refresh-tools", nil, nil)
	createVS := map[string]any{"slug": "agent", "name": "Agent", "authRequired": false, "tools": []any{map[string]any{"upstream": "fixture", "tool": "echo"}}}
	status, headers, vs := a.do("POST", "/api/v1/virtual-servers", createVS, nil)
	if status != 201 {
		t.Fatalf("create vs %d %+v", status, vs)
	}
	_, _, key := a.do("POST", "/api/v1/agent-keys", map[string]any{"slug": "agent-key", "virtualServers": []string{"agent"}}, nil)
	key["key"] = "redacted-example"
	examples["AgentKeyWithSecret"] = key
	_, _, admin := a.do("POST", "/api/v1/admin-tokens", map[string]any{"slug": "automation", "scope": "read"}, nil)
	readToken := admin["token"].(string)
	if status, _, _ := a.do("POST", "/api/v1/upstreams/fixture/connection-test", nil, map[string]string{"Authorization": "Bearer " + readToken}); status != 403 {
		t.Fatalf("read role accessed stored credentials: %d", status)
	}
	admin["token"] = "redacted-example"
	examples["AdminTokenWithSecret"] = admin
	examples["VirtualServerCreate"] = createVS
	examples["VirtualServer"] = vs
	if status, _, _ := a.do("PATCH", "/api/v1/virtual-servers/agent", map[string]any{"name": "Changed"}, map[string]string{"If-Match": headers.Get("ETag")}); status != 200 {
		t.Fatal("first editor rejected")
	}
	if status, _, _ := a.do("PATCH", "/api/v1/virtual-servers/agent", map[string]any{"name": "Lost update"}, map[string]string{"If-Match": headers.Get("ETag")}); status != 412 {
		t.Fatalf("stale editor allowed: %d", status)
	}
	if status, _, _ := a.do("POST", "/api/v1/admin-tokens/automation/rotate", nil, map[string]string{"If-Match": "\"999999\""}); status != 412 {
		t.Fatalf("stale token rotation allowed: %d", status)
	}
	_, _, exported := a.do("GET", "/api/v1/export", nil, nil)
	encoded, _ = json.Marshal(exported)
	if strings.Contains(string(encoded), "up-secret") || strings.Contains(string(encoded), "Authorization") {
		t.Fatal("export leaked secret")
	}
	if status, _, r := a.do("POST", "/api/v1/apply?dryRun=true&refreshTools=false", exported, nil); status != 200 {
		t.Fatalf("roundtrip %d %+v", status, r)
	}
	if status, _, r := a.do("POST", "/api/v1/apply?refreshTools=false", exported, nil); status != 200 {
		t.Fatalf("apply roundtrip %d %+v", status, r)
	}
	if status, _, r := a.do("POST", "/api/v1/upstreams/fixture/connection-test", nil, nil); status != 200 || r["ok"] != true {
		t.Fatalf("roundtrip lost stored credentials: %d %+v", status, r)
	}
	// Removed tools remain visible as orphaned selections instead of silently disappearing.
	if _, err := a.pool.Exec(a.t.Context(), `DELETE FROM upstream_tools WHERE name='echo'`); err != nil {
		t.Fatal(err)
	}
	_, _, orphan := a.do("GET", "/api/v1/virtual-servers/agent", nil, nil)
	if orphan["tools"].([]any)[0].(map[string]any)["orphaned"] != true {
		t.Fatal("orphan silently hidden")
	}
	if _, err := a.pool.Exec(a.t.Context(), `INSERT INTO upstreams(tenant_id,slug,name,url,transport) SELECT (SELECT id FROM tenants WHERE slug='default'),'bulk-'||lpad(i::text,3,'0'),'Bulk','https://example.com/mcp','streamable_http' FROM generate_series(1,205) i`); err != nil {
		t.Fatal(err)
	}
	_, _, first := a.do("GET", "/api/v1/upstreams?limit=200", nil, nil)
	if len(first["items"].([]any)) != 200 {
		t.Fatal("first page missing records")
	}
	cursor := first["nextCursor"].(string)
	_, _, second := a.do("GET", "/api/v1/upstreams?limit=200&cursor="+cursor, nil, nil)
	if len(second["items"].([]any)) != 6 {
		t.Fatal("remaining records unavailable")
	}
	if path := os.Getenv("VMCP_CAPTURE_API_EXAMPLES"); path != "" {
		b, _ := json.MarshalIndent(examples, "", "  ")
		if err := os.WriteFile(path, append(b, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
}
