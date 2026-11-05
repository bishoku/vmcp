package controlplane_test

import (
	"context"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/audit"
)

func controlAuditFixture(t *testing.T) (*api, string, string) {
	t.Helper()
	a := setup(t)
	for _, team := range []string{"before", "after"} {
		a.expect(200, "POST", "/api/v1/teams", map[string]any{"slug": team, "name": team, "issuer": "https://idp", "group": team})
	}
	a.expect(200, "POST", "/api/v1/projects", map[string]any{"slug": "product", "name": "Product", "ownerTeam": "before"})
	a.expect(201, "POST", "/api/v1/virtual-servers", map[string]any{"slug": "chat", "name": "Chat", "project": "product"})
	a.expect(201, "POST", "/api/v1/upstreams", map[string]any{"slug": "crm", "name": "CRM", "url": a.fixture.URL, "ownerTeam": "before", "headers": map[string]string{"Authorization": "Bearer up-secret"}})
	var before, after string
	if err := a.pool.QueryRow(context.Background(), `SELECT id::text FROM teams WHERE slug='before'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := a.pool.QueryRow(context.Background(), `SELECT id::text FROM teams WHERE slug='after'`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	return a, before, after
}
func awaitControlEvent(t *testing.T, a *api, op, stage string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var id string
		err := a.pool.QueryRow(context.Background(), `SELECT event_id::text FROM audit_events WHERE operation_id=$1 AND stage=$2`, op, stage).Scan(&id)
		if err == nil {
			return id
		}
		if time.Now().After(deadline) {
			t.Fatal("audit event did not persist", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func controlPayloadRead(t *testing.T, a *api, tok, id string, want int) {
	t.Helper()
	code, h, body := a.do("GET", "/api/v1/audit-events/"+id+"/payload", nil, map[string]string{"Authorization": "Bearer " + tok})
	if code != want || h.Get("Cache-Control") != "no-store" {
		t.Fatal("payload access", code, want, body, h)
	}
}
func TestControlAuditScopedRequestResponsePermissions(t *testing.T) {
	for _, mode := range []string{"strict", "relax"} {
		t.Run(mode, func(t *testing.T) {
			a, before, after := controlAuditFixture(t)
			ctx := context.Background()
			tok, id := payloadReader(t, a)
			settings := settingsFor(t, a)
			settings.Mode = mode
			settings.Scopes["control"] = audit.Capture{Request: true, Response: true}
			settings.Scopes["reads"] = audit.Capture{Response: true}
			a.expect(200, "PUT", "/api/v1/audit-settings", settings)
			for _, path := range []string{"/api/v1/virtual-servers/chat", "/api/v1/upstreams/crm"} {
				code, headers, result := a.do("PATCH", path, map[string]any{"description": "private-description"}, nil)
				if code != 200 {
					t.Fatal("resource patch failed", path, code, result)
				}
				op := headers.Get("X-Request-Id")
				if !audit.ValidID(op) {
					t.Fatal("operation correlation missing")
				}
				request := awaitControlEvent(t, a, op, "admission")
				response := awaitControlEvent(t, a, op, "outcome")
				for _, eid := range []string{request, response} {
					access, err := a.control.Store.AuditPayloadAccess(ctx, eid)
					if err != nil || access.Classification != "resource" || len(access.Domains) != 1 || access.Domains[0].OwnerTeam != before {
						t.Fatal("control producer context", access, err)
					}
				}
				// Create one role and reuse its assignment for each resource in this team.
				var roles int
				if err := a.pool.QueryRow(ctx, `SELECT count(*) FROM authorization_roles WHERE slug='payload-test'`).Scan(&roles); err != nil {
					t.Fatal(err)
				}
				if roles == 0 {
					payloadGrant(t, a, id, []string{"audit.events.read", "audit.payloads.request.read"}, "team", before, nil)
				}
				if _, err := a.pool.Exec(ctx, `UPDATE authorization_assignments SET capture_scopes=ARRAY['control'],scope_id=$1`, before); err != nil {
					t.Fatal(err)
				}
				if _, err := a.pool.Exec(ctx, `DELETE FROM authorization_role_permissions WHERE permission_id='audit.payloads.response.read' AND role_id IN(SELECT id FROM authorization_roles WHERE slug='payload-test')`); err != nil {
					t.Fatal(err)
				}
				if _, err := a.pool.Exec(ctx, `INSERT INTO authorization_role_permissions SELECT id,'audit.payloads.request.read' FROM authorization_roles WHERE slug='payload-test' ON CONFLICT DO NOTHING`); err != nil {
					t.Fatal(err)
				}

				controlPayloadRead(t, a, tok, request, 200)
				controlPayloadRead(t, a, tok, response, 404)
				// Direction is independent: response-only must not inherit request access.
				if _, err := a.pool.Exec(ctx, `DELETE FROM authorization_role_permissions WHERE permission_id='audit.payloads.request.read' AND role_id IN(SELECT id FROM authorization_roles WHERE slug='payload-test'); INSERT INTO authorization_role_permissions SELECT id,'audit.payloads.response.read' FROM authorization_roles WHERE slug='payload-test'`); err != nil {
					t.Fatal(err)
				}
				controlPayloadRead(t, a, tok, request, 404)
				controlPayloadRead(t, a, tok, response, 200)
				if _, err := a.pool.Exec(ctx, `UPDATE authorization_assignments SET scope_id=$1`, after); err != nil {
					t.Fatal(err)
				}
				controlPayloadRead(t, a, tok, response, 404)
				if _, err := a.pool.Exec(ctx, `UPDATE authorization_assignments SET scope_id=$1,capture_scopes=ARRAY['virtual']`, before); err != nil {
					t.Fatal(err)
				}
				controlPayloadRead(t, a, tok, response, 404) // runtime scope cannot open control bodies
				if _, err := a.pool.Exec(ctx, `UPDATE authorization_assignments SET capture_scopes=ARRAY['control']`); err != nil {
					t.Fatal(err)
				}
				_, readHeaders, _ := a.do("GET", path, nil, nil)
				readResponse := awaitControlEvent(t, a, readHeaders.Get("X-Request-Id"), "outcome")
				controlPayloadRead(t, a, tok, readResponse, 404)
				if _, err := a.pool.Exec(ctx, `UPDATE authorization_assignments SET capture_scopes=ARRAY['reads']`); err != nil {
					t.Fatal(err)
				}
				controlPayloadRead(t, a, tok, readResponse, 200)
				var mismatched int
				if err := a.pool.QueryRow(ctx, `SELECT count(*) FROM audit_events e JOIN audit_access_contexts c USING(event_id) WHERE e.operation_id=$1 AND e.config_revision<>c.operation_config_revision`, op).Scan(&mismatched); err != nil || mismatched != 0 {
					t.Fatal("mutation revision/context mismatch", mismatched, err)
				}
			}
		})
	}
}

func TestControlAuditPlatformAndTransferPayloadsStayTenantScoped(t *testing.T) {
	a, before, _ := controlAuditFixture(t)
	ctx := context.Background()
	tok, id := payloadReader(t, a)
	settings := settingsFor(t, a)
	settings.Scopes["control"] = audit.Capture{Request: true, Response: true}
	settings.Scopes["upstream"] = audit.Capture{Request: true, Response: true}
	a.expect(200, "PUT", "/api/v1/audit-settings", settings)
	payloadGrant(t, a, id, []string{"audit.events.read", "audit.payloads.request.read", "audit.payloads.response.read"}, "team", before, nil)
	if _, err := a.pool.Exec(ctx, `UPDATE authorization_assignments SET capture_scopes=ARRAY['control']`); err != nil {
		t.Fatal(err)
	}

	for _, call := range []struct {
		path string
		body any
		want int
	}{
		{"/api/v1/agent-keys", map[string]any{"slug": "agent", "virtualServers": []string{"chat"}}, 201},
		{"/api/v1/connection-tests", map[string]any{"url": a.fixture.URL}, 200},
		{"/api/v1/upstreams/crm/connection-test", nil, 200},
		{"/api/v1/virtual-servers", map[string]any{"slug": "created", "name": "Created", "project": "product"}, 201},
	} {
		code, h, body := a.do("POST", call.path, call.body, nil)
		if code != call.want {
			t.Fatal(call.path, code, body)
		}
		for _, stage := range []string{"admission", "outcome"} {
			eid := awaitControlEvent(t, a, h.Get("X-Request-Id"), stage)
			access, err := a.control.Store.AuditPayloadAccess(ctx, eid)
			if err != nil || access.Classification != "tenant" {
				t.Fatal("platform payload scoped to team", call.path, access, err)
			}
			controlPayloadRead(t, a, tok, eid, 404)
		}
		if call.path == "/api/v1/upstreams/crm/connection-test" {
			var attempts, scoped int
			if err := a.pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE c.classification<>'tenant') FROM audit_events e LEFT JOIN audit_access_contexts c USING(event_id) WHERE e.parent_operation_id=$1 AND e.scope='upstream'`, h.Get("X-Request-Id")).Scan(&attempts, &scoped); err != nil || attempts == 0 || scoped != 0 {
				t.Fatal("network attempt context lost", attempts, scoped, err)
			}
		}

	}
	// Case-insensitive ownership keys must not bypass transfer classification.
	code, h, body := a.do("PATCH", "/api/v1/virtual-servers/chat", map[string]any{"Project": "default"}, nil)
	if code != 200 {
		t.Fatal(code, body)
	}
	for _, stage := range []string{"admission", "outcome"} {
		eid := awaitControlEvent(t, a, h.Get("X-Request-Id"), stage)
		access, err := a.control.Store.AuditPayloadAccess(ctx, eid)
		if err != nil || access.Classification != "tenant" {
			t.Fatal("transfer payload exposed to a single side", access, err)
		}
		controlPayloadRead(t, a, tok, eid, 404)
	}
	// A tenant investigator can read it without legacy permissions, direction by direction.
	if _, err := a.pool.Exec(ctx, `UPDATE authorization_assignments SET scope_kind='tenant',scope_id=NULL`); err != nil {
		t.Fatal(err)
	}
	controlPayloadRead(t, a, tok, awaitControlEvent(t, a, h.Get("X-Request-Id"), "admission"), 200)
	controlPayloadRead(t, a, tok, awaitControlEvent(t, a, h.Get("X-Request-Id"), "outcome"), 200)
	// Standard mandatory secret masking still applies to captured token creation.
	var secretEvent string
	if err := a.pool.QueryRow(ctx, `SELECT event_id::text FROM audit_events WHERE action='POST /api/v1/agent-keys' AND stage='outcome' ORDER BY id DESC LIMIT 1`).Scan(&secretEvent); err != nil {
		t.Fatal(err)
	}
	_, _, payload := a.do("GET", "/api/v1/audit-events/"+secretEvent+"/payload", nil, map[string]string{"Authorization": "Bearer " + tok})
	content, ok := payload["content"].(map[string]any)
	if !ok || content["key"] != "[REDACTED]" {
		t.Fatal("issued key leaked", payload)
	}

}

func TestControlAuditConcurrentTransferCannotRelabelResponse(t *testing.T) {
	a, before, after := controlAuditFixture(t)
	ctx := context.Background()
	tok, id := payloadReader(t, a)
	settings := settingsFor(t, a)
	settings.Scopes["reads"] = audit.Capture{Response: true}
	a.expect(200, "PUT", "/api/v1/audit-settings", settings)
	payloadGrant(t, a, id, []string{"audit.events.read", "audit.payloads.response.read"}, "team", before, nil)
	if _, err := a.pool.Exec(ctx, `UPDATE authorization_assignments SET capture_scopes=ARRAY['reads']`); err != nil {
		t.Fatal(err)
	}

	// Deterministic race: transfer after scope pin/admission, before the handler's
	// resource read. The read returns the new owner's data, not the old snapshot.
	if _, err := a.pool.Exec(ctx, `CREATE FUNCTION transfer_on_read_admission() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.action='GET /api/v1/virtual-servers/{slug}' AND NEW.stage='admission' THEN
 UPDATE projects SET owner_team_id=(SELECT id FROM teams WHERE slug='after') WHERE slug='product';
 UPDATE virtual_servers SET description='new-owner-private-data' WHERE slug='chat';
 END IF; RETURN NEW; END $$;
 CREATE TRIGGER transfer_on_read_admission AFTER INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION transfer_on_read_admission()`); err != nil {
		t.Fatal(err)
	}
	code, h, body := a.do("GET", "/api/v1/virtual-servers/chat", nil, nil)
	if code != 412 || body["description"] == "new-owner-private-data" {
		t.Fatal("race fixture did not read new state", code, body)
	}
	eid := awaitControlEvent(t, a, h.Get("X-Request-Id"), "outcome")
	access, err := a.control.Store.AuditPayloadAccess(ctx, eid)
	if err != nil || access.Classification != "tenant" {
		t.Fatal("mixed-era response was scoped to old owner", access, err)
	}
	controlPayloadRead(t, a, tok, eid, 404)
	if _, err = a.pool.Exec(ctx, `UPDATE authorization_assignments SET scope_id=$1`, after); err != nil {
		t.Fatal(err)
	}
	controlPayloadRead(t, a, tok, eid, 404)
	if _, err = a.pool.Exec(ctx, `DROP TRIGGER transfer_on_read_admission ON audit_events`); err != nil {
		t.Fatal(err)
	}
	_, nextHeaders, _ := a.do("GET", "/api/v1/virtual-servers/chat", nil, nil)
	controlPayloadRead(t, a, tok, awaitControlEvent(t, a, nextHeaders.Get("X-Request-Id"), "outcome"), 200)
}

func TestControlAuditStaleScopedMutationRejected(t *testing.T) {
	a, _, _ := controlAuditFixture(t)
	ctx := context.Background()
	settings := settingsFor(t, a)
	settings.Scopes["control"] = audit.Capture{Request: true, Response: true}
	a.expect(200, "PUT", "/api/v1/audit-settings", settings)
	if _, err := a.pool.Exec(ctx, `CREATE FUNCTION transfer_on_write_admission() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.action='PATCH /api/v1/virtual-servers/{slug}' AND NEW.stage='admission' THEN
 UPDATE projects SET owner_team_id=(SELECT id FROM teams WHERE slug='after') WHERE slug='product';
 END IF; RETURN NEW; END $$;
 CREATE TRIGGER transfer_on_write_admission AFTER INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION transfer_on_write_admission()`); err != nil {
		t.Fatal(err)
	}
	code, h, body := a.do("PATCH", "/api/v1/virtual-servers/chat", map[string]any{"description": "must-not-commit"}, nil)
	if code != 412 {
		t.Fatal("stale scoped write committed", code, body)
	}
	var description string
	if err := a.pool.QueryRow(ctx, `SELECT description FROM virtual_servers WHERE slug='chat'`).Scan(&description); err != nil || description == "must-not-commit" {
		t.Fatal("stale mutation changed resource", description, err)
	}
	access, err := a.control.Store.AuditPayloadAccess(ctx, awaitControlEvent(t, a, h.Get("X-Request-Id"), "outcome"))
	if err != nil || access.Classification != "tenant" {
		t.Fatal("stale failure response remained scoped", access, err)
	}
}
