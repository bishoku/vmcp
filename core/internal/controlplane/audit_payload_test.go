package controlplane_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bishoku/vmcp/core/internal/audit"
)

func settingsFor(t *testing.T, a *api) audit.Settings {
	t.Helper()
	_, body := a.expect(200, "GET", "/api/v1/audit-settings", nil)
	raw, _ := json.Marshal(body)
	var s audit.Settings
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	return s
}
func TestAuditSettingsPreviewAndPayloadAccess(t *testing.T) {
	a := setup(t)
	ctx := context.Background()
	s := settingsFor(t, a)
	s.Mode = "strict"
	s.Scopes["control"] = audit.Capture{Request: true, Response: true}
	s.Rules = []audit.Rule{{Name: "email", Pattern: `[A-Za-z]+@example\.com`}}
	s.VirtualServers["support"] = []audit.Rule{{Name: "customer", Pattern: `CUST-[0-9]+`}}
	s.Upstreams["crm"] = []audit.Rule{{Name: "employee", Pattern: `EMP-[0-9]+`}}
	a.expect(200, "PUT", "/api/v1/audit-settings", s)
	a.expect(409, "PUT", "/api/v1/audit-settings", s)
	s = settingsFor(t, a)
	s.Scopes["virtual"] = audit.Capture{Request: true, Response: true}
	_, preview := a.expect(200, "POST", "/api/v1/audit-preview", map[string]any{"settings": s, "scope": "virtual", "direction": "request", "virtualServer": "support", "upstream": "crm", "payload": map[string]any{"text": "person@example.com CUST-123 EMP-456", "token": "preview-secret"}})
	raw, _ := json.Marshal(preview)
	for _, secret := range []string{"person@example.com", "CUST-123", "EMP-456", "preview-secret"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("preview leaked", secret)
		}
	}
	var n int
	if err := a.pool.QueryRow(ctx, `SELECT count(*) FROM audit_payloads p JOIN audit_events a USING(event_id) WHERE a.action='POST /api/v1/audit-preview'`).Scan(&n); err != nil || n != 0 {
		t.Fatal("preview sample was persisted", n, err)
	}
	a.expect(201, "POST", "/api/v1/admin-tokens", map[string]any{"slug": "payload-secret-test", "scope": "admin"})
	var id string
	if err := a.pool.QueryRow(ctx, `SELECT event_id::text FROM audit_events WHERE action='POST /api/v1/admin-tokens' AND stage='outcome'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	_, content := a.expect(200, "GET", "/api/v1/audit-events/"+id+"/payload", nil)
	raw, _ = json.Marshal(content)
	if strings.Contains(string(raw), "vmcp_admin_") || !strings.Contains(string(raw), "REDACTED") {
		t.Fatal("issued secret persisted", string(raw))
	}
	_, reader := a.expect(201, "POST", "/api/v1/admin-tokens", map[string]any{"slug": "payload-reader", "scope": "read"})
	for _, path := range []string{"/api/v1/audit-settings", "/api/v1/audit-events/" + id + "/payload"} {
		code, _, _ := a.do("GET", path, nil, map[string]string{"Authorization": "Bearer " + reader["token"].(string)})
		if code != 403 {
			t.Fatal("reader allowed payload policy access", code)
		}
	}
}
func TestControlPlaneRelaxAuditFailureDoesNotRollbackMutation(t *testing.T) {
	a := setup(t)
	ctx := context.Background()
	s := settingsFor(t, a)
	s.Mode = "relax"
	s.Scopes["control"] = audit.Capture{Request: true, Response: true}
	a.expect(200, "PUT", "/api/v1/audit-settings", s)
	if _, err := a.pool.Exec(ctx, `CREATE FUNCTION fail_all_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit rejected' USING ERRCODE='53100'; END $$; CREATE TRIGGER fail_all_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_all_audit()`); err != nil {
		t.Fatal(err)
	}
	a.expect(201, "POST", "/api/v1/admin-tokens", map[string]any{"slug": "relax-survives", "scope": "read"})
	var n int
	if err := a.pool.QueryRow(ctx, `SELECT count(*) FROM admin_tokens WHERE slug='relax-survives'`).Scan(&n); err != nil || n != 1 {
		t.Fatal("relax mutation lost", n, err)
	}
	// Remove the fault so cleanup can drain queued records within its normal budget.
	if _, err := a.pool.Exec(ctx, `DROP TRIGGER fail_all_audit ON audit_events`); err != nil {
		t.Fatal(err)
	}
}
func TestControlPlaneStrictPayloadFailureBlocksMutation(t *testing.T) {
	a := setup(t)
	ctx := context.Background()
	s := settingsFor(t, a)
	s.Mode = "strict"
	s.Scopes["control"] = audit.Capture{Request: true, Response: true}
	a.expect(200, "PUT", "/api/v1/audit-settings", s)
	if _, err := a.pool.Exec(ctx, `CREATE FUNCTION fail_payload_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'payload rejected' USING ERRCODE='53100'; END $$; CREATE TRIGGER fail_payload_audit BEFORE INSERT ON audit_payloads FOR EACH ROW EXECUTE FUNCTION fail_payload_audit()`); err != nil {
		t.Fatal(err)
	}
	a.expect(503, "POST", "/api/v1/admin-tokens", map[string]any{"slug": "strict-blocked", "scope": "read"})
	var n int
	_ = a.pool.QueryRow(ctx, `SELECT count(*) FROM admin_tokens WHERE slug='strict-blocked'`).Scan(&n)
	if n != 0 {
		t.Fatal("strict payload failure did not block mutation")
	}
}
