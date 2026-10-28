package controlplane_test

import (
	"context"
	"testing"
)

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

func TestBulkToolGrantAPIResultsAndAudit(t *testing.T) {
	a := setup(t)
	a.want("POST", "/api/v1/upstreams", map[string]any{"slug": "source", "name": "Source", "url": a.fixture.URL + "/mcp", "headers": map[string]string{"Authorization": "Bearer up-secret"}}, 201)
	a.want("POST", "/api/v1/upstreams/source/refresh-tools", nil, 200)
	a.want("PUT", "/api/v1/virtual-servers/chat", map[string]any{"name": "Chat", "tools": []any{map[string]string{"upstream": "source", "tool": "echo"}}}, 201)
	items := []any{map[string]any{"virtualServer": "chat", "upstream": "source", "tool": "echo", "status": "pending", "reason": "bulk request"}, map[string]any{"virtualServer": "chat", "upstream": "source", "tool": "missing", "status": "pending"}}
	result := a.want("POST", "/api/v1/tool-grants/bulk", map[string]any{"items": items}, 200)
	if result["changed"] != float64(1) || result["failed"] != float64(1) {
		t.Fatalf("partial results missing: %v", result)
	}
	grants := a.want("GET", "/api/v1/tool-grants", nil, 200)["items"].([]any)
	if len(grants) != 1 || grants[0].(map[string]any)["status"] != "approved" {
		t.Fatal("unmanaged request should auto approve")
	}
	var auditCount int
	if err := a.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_events WHERE target='tool-grant:chat/source/echo' AND stage='mutation'`).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("per-tool mutation audit missing: %d", auditCount)
	}

	if grants[0].(map[string]any)["consumerProject"] != "default" {
		t.Fatal("consumer project missing from review metadata")
	}
	// A mutation audit persistence failure must roll back just that item's decision.
	if _, err := a.pool.Exec(context.Background(), `CREATE FUNCTION reject_grant_mutation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.stage='mutation' AND NEW.target LIKE 'tool-grant:%' THEN RAISE EXCEPTION 'audit rejected' USING ERRCODE='53100'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_grant_mutation BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_grant_mutation()`); err != nil {
		t.Fatal(err)
	}
	failed := a.want("POST", "/api/v1/tool-grants/bulk", map[string]any{"items": []any{map[string]any{"virtualServer": "chat", "upstream": "source", "tool": "echo", "status": "revoked", "reason": "Rollback test", "version": 1}}}, 200)
	if failed["failed"] != float64(1) || failed["items"].([]any)[0].(map[string]any)["code"] != "audit_unavailable" {
		t.Fatalf("audit failure missing: %v", failed)
	}
	after := a.want("GET", "/api/v1/tool-grants", nil, 200)["items"].([]any)[0].(map[string]any)
	if after["status"] != "approved" || after["version"] != float64(1) {
		t.Fatalf("audit failure changed grant: %v", after)
	}
	a.want("POST", "/api/v1/tool-grants/bulk", map[string]any{"items": []any{}}, 422)
	a.want("POST", "/api/v1/tool-grants/bulk", map[string]any{"items": []any{items[0], items[0]}}, 422)
	status, _, _ := a.do("POST", "/api/v1/tool-grants/bulk", map[string]any{"items": items}, map[string]string{"Authorization": ""})
	if status != 401 {
		t.Fatalf("unauthenticated batch: %d", status)
	}
}
