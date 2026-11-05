package controlplane_test

import (
	"context"
	"fmt"
	"github.com/bishoku/vmcp/core/internal/audit"
	"testing"
)

func TestAuthorizationRoleManagement(t *testing.T) {
	a := setup(t)
	expect := func(want int, method, path string, body any, headers map[string]string) {
		t.Helper()
		status, _, result := a.do(method, path, body, headers)
		if status != want {
			t.Fatalf("%s %s: want %d got %d: %v", method, path, want, status, result)
		}
	}
	read, _ := payloadReader(t, a)
	for _, path := range []string{"/api/v1/permissions", "/api/v1/roles", "/api/v1/roles/platform-operator"} {
		expect(403, "GET", path, nil, map[string]string{"Authorization": "Bearer " + read})
	}
	expect(200, "GET", "/api/v1/permissions", nil, nil)
	_, _, builtin := a.do("GET", "/api/v1/roles/observer", nil, nil)
	expect(403, "PUT", "/api/v1/roles/observer", map[string]any{"name": "Changed", "permissions": []string{}}, map[string]string{"If-Match": "\"1\""})
	expect(403, "DELETE", "/api/v1/roles/platform-operator", nil, map[string]string{"If-Match": "\"1\""})
	in := map[string]any{"slug": "audit-custom", "name": "Custom auditor", "copyFrom": "observer"}
	status, h, role := a.do("POST", "/api/v1/roles", in, nil)
	if status != 201 || h.Get("ETag") != "\"1\"" || role["builtin"] != false {
		t.Fatalf("create: %d %v %v", status, h, role)
	}
	if fmt.Sprint(role["permissions"]) != fmt.Sprint(builtin["permissions"]) {
		t.Fatal("copy must freeze source permissions")
	}
	expect(409, "POST", "/api/v1/roles", in, nil)
	update := map[string]any{"name": "Payload investigator", "permissions": []string{"audit.events.read", "audit.payloads.request.read"}}
	expect(428, "PUT", "/api/v1/roles/audit-custom", update, nil)
	expect(428, "DELETE", "/api/v1/roles/audit-custom", nil, map[string]string{"If-Match": "*"})
	expect(412, "PUT", "/api/v1/roles/audit-custom", update, map[string]string{"If-Match": "\"9\""})
	status, h, _ = a.do("PUT", "/api/v1/roles/audit-custom", update, map[string]string{"If-Match": "\"1\""})
	if status != 200 || h.Get("ETag") != "\"2\"" {
		t.Fatalf("update: %d %v", status, h)
	}
	expect(412, "DELETE", "/api/v1/roles/audit-custom", nil, map[string]string{"If-Match": "\"1\""})
	expect(204, "DELETE", "/api/v1/roles/audit-custom", nil, map[string]string{"If-Match": "\"2\""})
	expect(404, "GET", "/api/v1/roles/audit-custom", nil, nil)
	for _, perms := range [][]string{{"invented.permission"}, {"audit.events.read", "audit.events.read"}} {
		expect(422, "POST", "/api/v1/roles", map[string]any{"slug": "invalid-role", "name": "Invalid", "permissions": perms}, nil)
	}
	expect(422, "POST", "/api/v1/roles", map[string]any{"slug": "invalid-role", "name": "Invalid", "builtin": true, "permissions": []string{}}, nil)
}

func TestAuthorizationRoleAssignmentGuards(t *testing.T) {
	a := setup(t)
	expect := func(want int, method, path string, body any, headers map[string]string) {
		t.Helper()
		status, _, result := a.do(method, path, body, headers)
		if status != want {
			t.Fatalf("%s %s: want %d got %d: %v", method, path, want, status, result)
		}
	}
	_, tokenID := payloadReader(t, a)
	_, _, role := a.do("POST", "/api/v1/roles", map[string]any{"slug": "assigned-role", "name": "Assigned", "permissions": []string{"audit.events.read"}}, nil)
	_, err := a.pool.Exec(context.Background(), `INSERT INTO authorization_assignments(role_id,principal_kind,principal_id,scope_kind,enabled) VALUES($1::uuid,'control_token',$2,'tenant',false)`, role["id"], tokenID)
	if err != nil {
		t.Fatal(err)
	}
	expect(409, "DELETE", "/api/v1/roles/assigned-role", nil, map[string]string{"If-Match": "\"1\""})
	expect(409, "PUT", "/api/v1/roles/assigned-role", map[string]any{"name": "Assigned", "permissions": []string{"audit.payloads.request.read"}}, map[string]string{"If-Match": "\"1\""})
	expect(200, "GET", "/api/v1/roles/assigned-role", nil, nil)
}

func TestAuthorizationRoleAuditAndConcurrentUpdates(t *testing.T) {
	a := setup(t)
	settings := settingsFor(t, a)
	settings.Scopes["control"] = audit.Capture{Request: true, Response: true}
	settings.Scopes["reads"] = audit.Capture{Request: true, Response: true}
	a.expect(200, "PUT", "/api/v1/audit-settings", settings)
	code, h, body := a.do("POST", "/api/v1/roles", map[string]any{"slug": "safe-audit", "name": "Sensitive name", "permissions": []string{}}, nil)
	if code != 201 {
		t.Fatal(code, body)
	}
	for _, stage := range []string{"admission", "mutation", "outcome"} {
		id := awaitControlEvent(t, a, h.Get("X-Request-Id"), stage)
		var payloads int
		if err := a.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_payloads WHERE event_id=$1`, id).Scan(&payloads); err != nil || payloads != 0 {
			t.Fatalf("management payload persisted: %d %v", payloads, err)
		}
	}
	statuses := make(chan int, 2)
	for i := 0; i < 2; i++ {
		go func() {
			code, _, _ := a.do("PUT", "/api/v1/roles/safe-audit", map[string]any{"name": "Concurrent", "permissions": []string{"audit.events.read"}}, map[string]string{"If-Match": "\"1\""})
			statuses <- code
		}()
	}
	one, two := <-statuses, <-statuses
	if !((one == 200 && two == 412) || (one == 412 && two == 200)) {
		t.Fatalf("concurrent updates: %d %d", one, two)
	}
	var selectors int
	if err := a.pool.QueryRow(context.Background(), `SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='authorization_state' AND column_name IN ('mode','audit_payload_mode')`).Scan(&selectors); err != nil || selectors != 0 {
		t.Fatal("removed mode selectors returned", selectors, err)
	}
	code, h, _ = a.do("GET", "/api/v1/roles", nil, nil)
	if code != 200 || h.Get("Cache-Control") != "no-store" {
		t.Fatal("management caching", code, h)
	}
	id := awaitControlEvent(t, a, h.Get("X-Request-Id"), "outcome")
	var payloads int
	if err := a.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_payloads WHERE event_id=$1`, id).Scan(&payloads); err != nil || payloads != 0 {
		t.Fatal("read response captured", payloads, err)
	}
}
