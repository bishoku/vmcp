package controlplane_test

import (
	"context"
	"github.com/bishoku/vmcp/core/internal/audit"
	"testing"
)

func TestConfigurationSelfAccessUsesLiveSourceAndFieldPolicy(t *testing.T) {
	a, team, _ := controlAuditFixture(t)
	token, id := payloadReader(t, a)
	configurationGrant(t, a, id, []string{"upstreams.read", "upstreams.update"}, "team", team, nil)

	headers := map[string]string{"Authorization": "Bearer " + token}
	checks := []any{
		map[string]any{"method": "GET", "path": "/upstreams/crm"},
		map[string]any{"method": "PATCH", "path": "/upstreams/crm", "body": map[string]any{"name": "Allowed"}},
		map[string]any{"method": "PATCH", "path": "/upstreams/crm", "body": map[string]any{"url": "https://private.invalid"}},
		map[string]any{"method": "GET", "path": "/upstreams/crm", "permission": "upstreams.credentials.update"},
		map[string]any{"method": "GET", "path": "/roles"},
		map[string]any{"method": "GET", "path": "/upstreams/hidden"},
		map[string]any{"method": "GET", "path": "https://external.invalid/upstreams/crm"},
		map[string]any{"method": "GET", "path": "/upstreams/crm", "permission": "access.roles.manage"},
	}
	code, h, out := a.do("POST", "/api/v1/configuration-access", map[string]any{"checks": checks}, headers)
	if code != 200 || h.Get("Cache-Control") != "no-store" {
		t.Fatal(code, h, out)
	}
	allowed := out["allowed"].([]any)
	for i, want := range []bool{true, true, false, false, false, false, false, false} {
		if allowed[i] != want {
			t.Fatal(i, allowed)
		}
	}
	if _, exists := out["mode"]; exists {
		t.Fatal("removed mode leaked", out)
	}
	code, _, me := a.do("GET", "/api/v1/me", nil, headers)
	if code != 200 || me["configurationMode"] != nil {
		t.Fatal(code, me)
	}
	// Inspection must not execute even an allowed mutation.
	code, _, current := a.do("GET", "/api/v1/upstreams/crm", nil, headers)
	if code != 200 || current["name"] == "Allowed" {
		t.Fatal("capability check mutated data", current)
	}
	if _, err := a.pool.Exec(context.Background(), `UPDATE authorization_assignments SET enabled=false WHERE principal_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	code, _, out = a.do("POST", "/api/v1/configuration-access", map[string]any{"checks": checks}, headers)
	if code != 200 {
		t.Fatal(code, out)
	}
	for _, v := range out["allowed"].([]any) {
		if v != false {
			t.Fatal("revoked capabilities survived", out)
		}
	}
	code, _, _ = a.do("POST", "/api/v1/configuration-access", map[string]any{"principal": map[string]string{"id": "bootstrap"}, "checks": checks}, headers)
	if code != 422 {
		t.Fatal("caller identity injection accepted", code)
	}
}
func TestConfigurationSelfAccessDoesNotCaptureProbePayloads(t *testing.T) {
	a := setup(t)
	settings := settingsFor(t, a)
	settings.Mode = "strict"
	settings.Scopes["control"] = audit.Capture{Request: true, Response: true}
	a.expect(200, "PUT", "/api/v1/audit-settings", settings)

	headers, out := a.expect(200, "POST", "/api/v1/configuration-access", map[string]any{"checks": []any{map[string]any{"method": "POST", "path": "/upstreams", "body": map[string]any{"slug": "probe-only", "url": a.fixture.URL, "headers": map[string]string{"Authorization": "probe-secret"}}}}})
	if out["allowed"].([]any)[0] != true {
		t.Fatal(out)
	}
	op := headers.Get("X-Request-Id")
	awaitControlEvent(t, a, op, "outcome")
	var count, payloads int
	if err := a.pool.QueryRow(context.Background(), `SELECT count(*),count(p.event_id) FROM audit_events e LEFT JOIN audit_payloads p USING(event_id) WHERE e.operation_id=$1`, op).Scan(&count, &payloads); err != nil || count == 0 || payloads != 0 {
		t.Fatal("probe payload persisted", count, payloads, err)
	}
	code, _, _ := a.do("GET", "/api/v1/upstreams/probe-only", nil, nil)
	if code != 404 {
		t.Fatal("probe executed a create", code)
	}
}

func TestConfigurationSelfAccessPreservesTokenRotationIdentityBoundary(t *testing.T) {
	a := setup(t)
	token, id := payloadReader(t, a)
	a.expect(201, "POST", "/api/v1/admin-tokens", map[string]any{"slug": "privileged", "scope": "admin"})
	a.expect(201, "POST", "/api/v1/admin-tokens", map[string]any{"slug": "other-assigned", "scope": "read"})
	configurationGrant(t, a, id, []string{"control_tokens.rotate"}, "tenant", nil, nil)
	if _, err := a.pool.Exec(context.Background(), `INSERT INTO authorization_assignments(role_id,principal_kind,principal_id,scope_kind) SELECT role_id,'control_token',(SELECT id::text FROM admin_tokens WHERE slug='other-assigned'),'tenant' FROM authorization_assignments WHERE principal_id=$1`, id); err != nil {
		t.Fatal(err)
	}

	var self string
	if err := a.pool.QueryRow(context.Background(), `SELECT slug FROM admin_tokens WHERE id::text=$1`, id).Scan(&self); err != nil {
		t.Fatal(err)
	}
	checks := []any{}
	for _, slug := range []string{"privileged", "other-assigned", self} {
		checks = append(checks, map[string]any{"method": "GET", "path": "/admin-tokens/" + slug, "permission": "control_tokens.rotate"}, map[string]any{"method": "POST", "path": "/admin-tokens/" + slug + "/rotate"})
	}
	code, _, out := a.do("POST", "/api/v1/configuration-access", map[string]any{"checks": checks}, map[string]string{"Authorization": "Bearer " + token})
	if code != 200 {
		t.Fatal(code, out)
	}
	for i, want := range []bool{false, false, false, false, true, true} {
		if out["allowed"].([]any)[i] != want {
			t.Fatal("token rotation escaped identity boundary", out)
		}
	}
}
