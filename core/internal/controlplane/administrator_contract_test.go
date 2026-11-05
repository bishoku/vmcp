package controlplane_test

import (
	"context"
	"strings"
	"testing"
)

func TestPlatformOperatorIsNotAdministrator(t *testing.T) {
	a := setup(t)
	secret, id := payloadReader(t, a)
	a.expect(201, "POST", "/api/v1/admin-tokens", map[string]any{"slug": "privileged-target", "scope": "admin"})
	role := assignmentCall(t, a, 200, "GET", "/api/v1/roles/platform-operator", nil, "")
	for _, raw := range role["permissions"].([]any) {
		if strings.HasPrefix(raw.(string), "access.") {
			t.Fatal("operator template includes management permission", raw)
		}
	}
	grant := assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", assignmentInput(role["id"].(string), id, map[string]any{"kind": "tenant"}), "")
	headers := map[string]string{"Authorization": "Bearer " + secret}
	code, _, me := a.do("GET", "/api/v1/me", nil, headers)
	if code != 200 || me["role"] == "admin" {
		t.Fatal("assignment changed identity", code, me)
	}
	for _, path := range []string{"/api/v1/roles", "/api/v1/role-assignments", "/api/v1/identity-mappings", "/api/v1/access/status"} {
		code, _, _ = a.do("GET", path, nil, headers)
		if code != 403 {
			t.Fatal("operator accessed management", path, code)
		}
	}
	// Broad operational permissions cannot mint or promote an administrator token.
	for _, check := range []struct {
		method, path string
		body         any
	}{
		{"POST", "/api/v1/admin-tokens", map[string]any{"slug": "escalated", "scope": "admin"}},
		{"PATCH", "/api/v1/admin-tokens/payload-investigator", map[string]any{"scope": "admin"}},
		{"POST", "/api/v1/admin-tokens/privileged-target/rotate", nil},
	} {
		code, _, _ = a.do(check.method, check.path, check.body, headers)
		if code != 403 {
			t.Fatal("operator escalated token identity", check.path, code)
		}
	}
	for _, direction := range []string{"request", "response"} {
		event := payloadEvent(t, a, direction)
		code, _, _ = a.do("GET", "/api/v1/audit-events/"+event+"/payload", nil, headers)
		if code != 200 {
			t.Fatal("operator lost explicitly assigned payload scope", direction, code)
		}
	}
	assignmentCall(t, a, 204, "DELETE", "/api/v1/role-assignments/"+grant["id"].(string), nil, "\"1\"")
	code, _, _ = a.do("GET", "/api/v1/upstreams", nil, headers)
	if code != 403 {
		t.Fatal("operator rights survived revoke", code)
	}
	// Even an explicitly custom permission bundle cannot grant admin identity.
	custom := assignmentRole(t, a, "platform-admin", "access.roles.manage", "access.assignments.manage", "access.mappings.manage")
	assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", assignmentInput(custom, id, map[string]any{"kind": "tenant"}), "")
	code, _, _ = a.do("POST", "/api/v1/roles", map[string]any{"slug": "escalation", "name": "Escalation", "permissions": []string{}}, headers)
	if code != 403 {
		t.Fatal("custom access permission created administrator", code)
	}
}

func TestBootstrapRecoveryWithoutAssignmentsOrNamedAdministrator(t *testing.T) {
	a := setup(t) // token-only mode is independent of OIDC discovery/login.
	_, created := a.expect(201, "POST", "/api/v1/admin-tokens", map[string]any{"slug": "last-named-admin", "scope": "admin"})
	adminSecret := created["token"].(string)
	code, _, me := a.do("GET", "/api/v1/me", nil, map[string]string{"Authorization": "Bearer " + adminSecret})
	if code != 200 || me["role"] != "admin" {
		t.Fatal("named administrator not recognized", code, me)
	}
	// Bootstrap remains the recovery root, so the final named admin is revocable.
	a.expect(200, "POST", "/api/v1/admin-tokens/last-named-admin/revoke", nil)
	code, _, _ = a.do("GET", "/api/v1/roles", nil, map[string]string{"Authorization": "Bearer " + adminSecret})
	if code != 401 {
		t.Fatal("revoked administrator remained valid", code)
	}
	var assignments int
	if err := a.pool.QueryRow(context.Background(), `SELECT count(*) FROM authorization_assignments`).Scan(&assignments); err != nil || assignments != 0 {
		t.Fatal("recovery fixture has assignments", assignments, err)
	}
	code, _, me = a.do("GET", "/api/v1/me", nil, nil)
	if code != 200 || me["role"] != "admin" || me["via"] != "bootstrap-token" {
		t.Fatal("bootstrap recovery identity", code, me)
	}
	code, recoveryHeaders, _ := a.do("POST", "/api/v1/roles", map[string]any{"slug": "recovered-role", "name": "Recovered role", "permissions": []string{"upstreams.read"}}, nil)
	if code != 201 {
		t.Fatal("bootstrap could not repair access", code)
	}
	event := awaitControlEvent(t, a, recoveryHeaders.Get("X-Request-Id"), "outcome")
	var actorType, actorID, record string
	if err := a.pool.QueryRow(context.Background(), `SELECT actor_type,actor_id,to_jsonb(e)::text FROM audit_events e WHERE event_id=$1`, event).Scan(&actorType, &actorID, &record); err != nil || actorType != "bootstrap-token" || actorID != "bootstrap" || strings.Contains(record, token) {
		t.Fatal("recovery audit identity/privacy", actorType, actorID, err)
	}
	for _, direction := range []string{"request", "response"} {
		event := payloadEvent(t, a, direction)
		code, _, _ = a.do("GET", "/api/v1/audit-events/"+event+"/payload", nil, nil)
		if code != 200 {
			t.Fatal("administrator payload authority", direction, code)
		}
	}
	// Simulate deployment secret replacement; old bootstrap no longer authenticates.
	a.control.AdminToken = "replacement-bootstrap-secret"
	code, _, _ = a.do("GET", "/api/v1/roles", nil, nil)
	if code != 401 {
		t.Fatal("old bootstrap survived replacement", code)
	}
	code, _, _ = a.do("GET", "/api/v1/roles", nil, map[string]string{"Authorization": "Bearer replacement-bootstrap-secret"})
	if code != 200 {
		t.Fatal("replacement bootstrap failed", code)
	}
}

func TestOIDCAdministratorDoesNotDependOnPermissionAssignments(t *testing.T) {
	a, idp := claimSessionAPI(t)
	cookie := claimSessionCookie(t, a, idp.URL(), "admin", map[string]any{}, true)
	headers := cookieHeaders(cookie)
	code, _, _ := a.do("GET", "/api/v1/roles", nil, headers)
	if code != 200 {
		t.Fatal("authenticated OIDC administrator lost management", code)
	}
	for _, direction := range []string{"request", "response"} {
		event := payloadEvent(t, a, direction)
		code, _, _ = a.do("GET", "/api/v1/audit-events/"+event+"/payload", nil, headers)
		if code != 200 {
			t.Fatal("OIDC administrator payload authority", direction, code)
		}
	}
	code, _, _ = a.do("POST", "/api/v1/roles", map[string]any{"slug": "csrf-rejected", "name": "Rejected", "permissions": []string{}}, headers)
	if code != 403 {
		t.Fatal("administrator bypassed CSRF", code)
	}
}
