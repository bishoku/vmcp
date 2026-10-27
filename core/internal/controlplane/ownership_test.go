package controlplane_test

import "testing"

func TestOwnershipAPIRoundTripAndDefaultCompatibility(t *testing.T) {
	a := setup(t)
	a.want("POST", "/api/v1/teams", map[string]any{"slug": "crm", "name": "CRM owners", "issuer": "https://idp.example", "group": "crm-owners"}, 200)
	a.want("POST", "/api/v1/projects", map[string]any{"slug": "chat", "name": "Customer chat", "ownerTeam": "crm"}, 200)
	a.want("POST", "/api/v1/upstreams", map[string]any{"slug": "owned", "name": "CRM", "url": a.fixture.URL + "/mcp", "ownerTeam": "crm", "headers": map[string]string{"Authorization": "Bearer up-secret"}}, 201)
	a.want("POST", "/api/v1/upstreams/owned/refresh-tools", nil, 200)
	v := a.want("PUT", "/api/v1/virtual-servers/chat", map[string]any{"name": "Chat", "project": "chat", "tools": []any{map[string]string{"upstream": "owned", "tool": "echo"}}}, 201)
	if v["project"] != "chat" || v["ownerTeam"] != "crm" {
		t.Fatalf("ownership missing: %v", v)
	}
	a.want("PATCH", "/api/v1/virtual-servers/chat", map[string]any{"description": "updated"}, 200)
	_, _, doc := a.do("GET", "/api/v1/export", nil, nil)
	if len(doc["teams"].([]any)) != 1 {
		t.Fatal("teams missing from export")
	}
	a.want("POST", "/api/v1/apply?refreshTools=false", doc, 200)
	v = a.want("GET", "/api/v1/virtual-servers/chat", nil, 200)
	if v["project"] != "chat" || v["ownerTeam"] != "crm" {
		t.Fatal("GitOps lost ownership")
	}
	a.want("POST", "/api/v1/projects", map[string]any{"slug": "default", "name": "Default", "ownerTeam": "crm", "version": 1}, 403)
	a.want("POST", "/api/v1/teams", map[string]any{"slug": "crm", "name": "changed", "issuer": "https://idp.example", "group": "crm-owners"}, 412)
}

func (a *api) want(method, path string, body any, status int) map[string]any {
	a.t.Helper()
	_, v := a.expect(status, method, path, body)
	return v
}
