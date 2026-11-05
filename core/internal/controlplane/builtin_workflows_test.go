package controlplane_test

import (
	"context"
	"testing"
)

func assignBuiltin(t *testing.T, a *api, token, slug, kind string, id any) {
	t.Helper()
	if _, err := a.pool.Exec(context.Background(), `INSERT INTO authorization_assignments(role_id,principal_kind,principal_id,scope_kind,scope_id) SELECT id,'control_token',$1,$2,$3 FROM authorization_roles WHERE slug=$4 AND builtin`, token, kind, id, slug); err != nil {
		t.Fatal(err)
	}
}
func TestBuiltinCredentialManagerScopedWorkflow(t *testing.T) {
	a, team, _ := controlAuditFixture(t)
	token, id := payloadReader(t, a)
	assignBuiltin(t, a, id, "credential-manager", "team", team)
	headers := map[string]string{"Authorization": "Bearer " + token}
	a.expect(200, "POST", "/api/v1/projects", map[string]any{"slug": "other", "name": "Other", "ownerTeam": "after"})
	a.expect(201, "POST", "/api/v1/virtual-servers", map[string]any{"slug": "outside", "name": "Outside", "project": "other"})
	code, _, out := a.do("GET", "/api/v1/virtual-servers", nil, headers)
	if code != 200 || len(out["items"].([]any)) != 1 || out["items"].([]any)[0].(map[string]any)["slug"] != "chat" {
		t.Fatal("target picker scope", code, out)
	}
	code, _, out = a.do("POST", "/api/v1/agent-keys", map[string]any{"slug": "managed", "virtualServers": []string{"chat"}}, headers)
	if code != 201 {
		t.Fatal("create scoped credential", code, out)
	}
	code, _, out = a.do("POST", "/api/v1/agent-keys/managed/rotate", nil, headers)
	if code != 200 {
		t.Fatal("rotate scoped credential", code, out)
	}
	code, _, _ = a.do("POST", "/api/v1/agent-keys", map[string]any{"slug": "cross", "virtualServers": []string{"chat", "outside"}}, headers)
	if code != 403 {
		t.Fatal("cross-team credential accepted", code)
	}
	code, _, _ = a.do("PATCH", "/api/v1/virtual-servers/chat", map[string]any{"name": "Forbidden"}, headers)
	if code != 403 {
		t.Fatal("picker read became write", code)
	}
}
func TestBuiltinVirtualServerEditorScopedWorkflow(t *testing.T) {
	a, team, _ := controlAuditFixture(t)
	token, id := payloadReader(t, a)
	assignBuiltin(t, a, id, "virtual-server-editor", "team", team)
	headers := map[string]string{"Authorization": "Bearer " + token}
	code, _, out := a.do("GET", "/api/v1/projects", nil, headers)
	if code != 200 || len(out["items"].([]any)) != 1 {
		t.Fatal("scoped project picker", code, out)
	}
	code, _, out = a.do("GET", "/api/v1/tool-catalog", nil, headers)
	if code != 200 {
		t.Fatal("shared catalog", code, out)
	}
	code, _, out = a.do("POST", "/api/v1/virtual-servers", map[string]any{"slug": "draft", "name": "Draft", "project": "product", "tools": []any{}}, headers)
	if code != 201 {
		t.Fatal("create draft", code, out)
	}
	for _, patch := range []map[string]any{{"enabled": true}, {"authRequired": false}, {"project": "default"}} {
		code, _, _ = a.do("PATCH", "/api/v1/virtual-servers/draft", patch, headers)
		if code != 403 {
			t.Fatal("draft editor gained protected field", patch, code)
		}
	}
	for _, path := range []string{"/api/v1/upstreams", "/api/v1/roles", "/api/v1/agent-keys"} {
		code, _, _ = a.do("GET", path, nil, headers)
		if code != 403 {
			t.Fatal("unrelated read", path, code)
		}
	}
}
func TestBuiltinAutomationResourceDeploymentAndBoundaries(t *testing.T) {
	a := setup(t)
	token, id := payloadReader(t, a)
	assignBuiltin(t, a, id, "automation-deployer", "tenant", nil)
	headers := map[string]string{"Authorization": "Bearer " + token}
	code, _, out := a.do("GET", "/api/v1/export", nil, headers)
	if code != 200 {
		t.Fatal("export dependencies", code, out)
	}
	doc := map[string]any{"apiVersion": "vmcp/v1", "upstreams": []any{map[string]any{"slug": "deployed", "name": "Deployed", "url": a.fixture.URL}}, "virtualServers": []any{}}
	for _, query := range []string{"dryRun=true&refreshTools=false", "refreshTools=false"} {
		code, _, out = a.do("POST", "/api/v1/apply?"+query, doc, headers)
		if code != 200 {
			t.Fatal("resource apply", query, code, out)
		}
	}
	code, _, _ = a.do("POST", "/api/v1/apply?prune=true&refreshTools=false", doc, headers)
	if code != 403 {
		t.Fatal("deployment became destructive prune", code)
	}
	for _, path := range []string{"/api/v1/roles", "/api/v1/admin-tokens", "/api/v1/audit-events"} {
		code, _, _ = a.do("GET", path, nil, headers)
		if code != 403 {
			t.Fatal("deployment gained unrelated data", path, code)
		}
	}
}
