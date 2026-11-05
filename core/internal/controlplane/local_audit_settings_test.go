package controlplane_test

import (
	"context"
	"github.com/bishoku/vmcp/core/internal/audit"
	"reflect"
	"testing"
)

func TestLocalAuditSettingsResourceScopeAndIsolation(t *testing.T) {
	for _, family := range []string{"upstreams", "virtual-servers"} {
		t.Run(family, func(t *testing.T) {
			a, team, _ := controlAuditFixture(t)
			a.expect(201, "POST", "/api/v1/upstreams", map[string]any{"slug": "outside", "name": "Outside", "url": a.fixture.URL, "ownerTeam": "after"})
			a.expect(200, "POST", "/api/v1/projects", map[string]any{"slug": "other", "name": "Other", "ownerTeam": "after"})
			a.expect(201, "POST", "/api/v1/virtual-servers", map[string]any{"slug": "outside", "name": "Outside", "project": "other"})
			settings := settingsFor(t, a)
			settings.Mode = "strict"
			settings.Scopes["upstream"] = audit.Capture{Request: true, Response: true}
			settings.Scopes["virtual"] = audit.Capture{Request: true, Response: true}
			settings.Scopes["control"] = audit.Capture{Request: true, Response: true}
			settings.Rules = []audit.Rule{{Name: "global", Pattern: "GLOBAL-[0-9]+"}}
			settings.Upstreams["outside"] = []audit.Rule{{Name: "private", Pattern: "HIDDEN"}}
			settings.VirtualServers["outside"] = []audit.Rule{{Name: "private", Pattern: "HIDDEN"}}
			_, saved := a.expect(200, "PUT", "/api/v1/audit-settings", settings)
			token, id := payloadReader(t, a)
			configurationGrant(t, a, id, []string{"audit.settings.read", "audit.redaction.local.update", "audit.redaction.preview"}, "team", team, nil)

			headers := map[string]string{"Authorization": "Bearer " + token}
			slug := "crm"
			if family == "virtual-servers" {
				slug = "chat"
			}
			path := "/api/v1/" + family + "/" + slug + "/audit-settings"
			code, h, local := a.do("GET", path, nil, headers)
			if code != 200 || h.Get("Cache-Control") != "no-store" || len(local) != 2 {
				t.Fatal(code, h, local)
			}
			for _, method := range []string{"GET", "PUT", "POST"} {
				suffix := ""
				var body any = map[string]any{"revision": local["revision"], "rules": []any{}}
				if method == "POST" {
					suffix = "/preview"
					body = map[string]any{"revision": local["revision"], "rules": []any{}, "direction": "request", "payload": map[string]string{"text": "HIDDEN"}}
				}
				code, _, out := a.do(method, "/api/v1/"+family+"/outside/audit-settings"+suffix, body, headers)
				if code != 403 {
					t.Fatal("cross-scope local settings", method, code, out)
				}
			}
			code, _, _ = a.do("GET", "/api/v1/audit-settings", nil, headers)
			if code != 403 {
				t.Fatal("global document exposed", code)
			}
			rules := []any{map[string]string{"name": "local", "pattern": "LOCAL-[0-9]+"}}
			code, _, out := a.do("POST", path+"/preview", map[string]any{"revision": local["revision"], "rules": rules, "direction": "request", "payload": map[string]string{"text": "GLOBAL-123 LOCAL-456"}}, headers)
			if code != 200 || out["redactions"].(float64) < 2 {
				t.Fatal("global protection omitted from preview", code, out)
			}
			code, _, _ = a.do("PUT", path, map[string]any{"revision": local["revision"], "rules": rules, "virtualServers": map[string]any{"outside": []any{}}}, headers)
			if code != 422 {
				t.Fatal("foreign rules accepted", code)
			}
			code, h, out = a.do("PUT", path, map[string]any{"revision": local["revision"], "rules": rules}, headers)
			if code != 200 || out["revision"].(float64) != local["revision"].(float64)+1 {
				t.Fatal(code, out)
			}
			_, all := a.expect(200, "GET", "/api/v1/audit-settings", nil)
			key := "upstreams"
			if family == "virtual-servers" {
				key = "virtualServers"
			}
			allMap := all[key].(map[string]any)
			oldMap := saved[key].(map[string]any)
			delete(allMap, slug)
			delete(oldMap, slug)
			delete(all, "revision")
			delete(saved, "revision")
			if !reflect.DeepEqual(all, saved) {
				t.Fatal("local save changed unrelated/global settings", all, saved)
			}
			code, _, _ = a.do("PUT", path, map[string]any{"revision": local["revision"], "rules": rules}, headers)
			if code != 409 {
				t.Fatal("stale revision overwrote settings", code)
			}
			op := h.Get("X-Request-Id")
			awaitControlEvent(t, a, op, "outcome")
			var payloads int
			if err := a.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_payloads p JOIN audit_events e USING(event_id) WHERE e.operation_id=$1`, op).Scan(&payloads); err != nil || payloads != 0 {
				t.Fatal("local settings captured", payloads, err)
			}
		})
	}
}
func TestLocalAuditIndependentPermissionsAndModes(t *testing.T) {
	for _, mode := range []string{"mandatory"} {
		t.Run(mode, func(t *testing.T) {
			a, team, _ := controlAuditFixture(t)
			token, id := payloadReader(t, a)
			configurationGrant(t, a, id, []string{"audit.settings.read"}, "team", team, nil)

			headers := map[string]string{"Authorization": "Bearer " + token}
			path := "/api/v1/upstreams/crm/audit-settings"
			code, _, local := a.do("GET", path, nil, headers)
			if code != 200 {
				t.Fatal(code, local)
			}
			code, _, _ = a.do("PUT", path, map[string]any{"revision": local["revision"], "rules": []any{}}, headers)
			if code != 403 {
				t.Fatal("read became update", code)
			}
			code, _, _ = a.do("POST", path+"/preview", map[string]any{"revision": local["revision"], "rules": []any{}, "direction": "request", "payload": map[string]string{}}, headers)
			if code != 403 {
				t.Fatal("read became preview", code)
			}
		})
	}
}
func TestLocalAuditWriteRechecksRevocation(t *testing.T) {
	a, team, _ := controlAuditFixture(t)
	token, id := payloadReader(t, a)
	configurationGrant(t, a, id, []string{"audit.settings.read", "audit.redaction.local.update"}, "team", team, nil)

	headers := map[string]string{"Authorization": "Bearer " + token}
	_, _, local := a.do("GET", "/api/v1/upstreams/crm/audit-settings", nil, headers)
	if _, err := a.pool.Exec(context.Background(), `CREATE FUNCTION revoke_local_before_write() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.stage='admission' AND NEW.method='PUT' AND NEW.action LIKE '%/audit-settings' THEN UPDATE authorization_assignments SET enabled=false WHERE role_id=(SELECT id FROM authorization_roles WHERE slug='config-test'); END IF; RETURN NEW; END $$; CREATE TRIGGER revoke_local_before_write AFTER INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION revoke_local_before_write()`); err != nil {
		t.Fatal(err)
	}
	code, _, out := a.do("PUT", "/api/v1/upstreams/crm/audit-settings", map[string]any{"revision": local["revision"], "rules": []any{map[string]string{"name": "blocked", "pattern": "SECRET"}}}, headers)
	if code != 412 {
		t.Fatal("stale authority committed", code, out)
	}
}

func TestLocalAuditTransferBetweenAdmissionAndRead(t *testing.T) {
	a, team, _ := controlAuditFixture(t)
	token, id := payloadReader(t, a)
	configurationGrant(t, a, id, []string{"audit.settings.read"}, "team", team, nil)

	if _, err := a.pool.Exec(context.Background(), `CREATE FUNCTION transfer_before_local_read() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.stage='admission' AND NEW.method='GET' AND NEW.action LIKE '%/audit-settings' THEN UPDATE upstreams SET owner_team_id=(SELECT id FROM teams WHERE slug='after') WHERE slug='crm'; END IF; RETURN NEW; END $$; CREATE TRIGGER transfer_before_local_read AFTER INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION transfer_before_local_read()`); err != nil {
		t.Fatal(err)
	}
	code, _, out := a.do("GET", "/api/v1/upstreams/crm/audit-settings", nil, map[string]string{"Authorization": "Bearer " + token})
	if code != 412 {
		t.Fatal("old owner received new scope settings", code, out)
	}
}

func TestLocalAuditPreviewDoesNotGrantWriteOrSavedPayload(t *testing.T) {
	a, team, _ := controlAuditFixture(t)
	token, id := payloadReader(t, a)
	configurationGrant(t, a, id, []string{"audit.settings.read", "audit.redaction.preview"}, "team", team, nil)

	headers := map[string]string{"Authorization": "Bearer " + token}
	_, _, local := a.do("GET", "/api/v1/upstreams/crm/audit-settings", nil, headers)
	code, _, out := a.do("POST", "/api/v1/upstreams/crm/audit-settings/preview", map[string]any{"revision": local["revision"], "rules": []any{}, "direction": "response", "payload": map[string]string{"example": "synthetic"}}, headers)
	if code != 200 {
		t.Fatal(code, out)
	}
	code, _, _ = a.do("PUT", "/api/v1/upstreams/crm/audit-settings", map[string]any{"revision": local["revision"], "rules": []any{}}, headers)
	if code != 403 {
		t.Fatal("preview became write", code)
	}
	code, _, _ = a.do("POST", "/api/v1/upstreams/crm/audit-settings/preview", map[string]any{"revision": local["revision"], "rules": []any{}, "direction": "request", "payload": map[string]string{}, "upstream": "outside"}, headers)
	if code != 422 {
		t.Fatal("preview context overridden", code)
	}
}
