package controlplane_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func visibilityEvent(t *testing.T, a *api, team, direction string) string {
	t.Helper()
	ctx := context.Background()
	id := payloadEvent(t, a, direction)
	var epoch, resource string
	if err := a.pool.QueryRow(ctx, `INSERT INTO audit_resource_epochs(resource_kind,resource_id,owner_team_id,start_revision) VALUES('virtual_server',gen_random_uuid(),$1,1) RETURNING epoch_id::text,resource_id::text`, team).Scan(&epoch, &resource); err != nil {
		t.Fatal(err)
	}
	if _, err := a.pool.Exec(ctx, `INSERT INTO audit_access_contexts(event_id,classification,origin,operation_config_revision,provenance_state) VALUES($1,'resource','virtual',1,'verified')`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := a.pool.Exec(ctx, `INSERT INTO audit_access_domains(event_id,domain_kind,resource_kind,resource_id,owner_team_id,epoch_id) VALUES($1,'consumer','virtual_server',$2,$3,$4)`, id, resource, team, epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := a.pool.Exec(ctx, `UPDATE audit_events SET actor_id='private-actor',issuer='private-issuer',agent_key='private-key',subject='private-subject',client_ip='private-ip',reason='private-reason',target='private-target',conversation_id='one',root_conversation_id='root',conversation_group_id='shared-group' WHERE event_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	return id
}
func TestAuditMetadataVisibilityAndEventCapabilities(t *testing.T) {
	a := setup(t)
	ctx := context.Background()
	tok, id := payloadReader(t, a)
	const teamA = "11111111-1111-1111-1111-111111111111"
	const teamB = "22222222-2222-2222-2222-222222222222"
	req := visibilityEvent(t, a, teamA, "request")
	resp := visibilityEvent(t, a, teamA, "response")
	hidden := visibilityEvent(t, a, teamB, "response") // newest hidden row cannot consume page slots
	payloadGrant(t, a, id, []string{"audit.events.read", "audit.payloads.request.read"}, "team", teamA, nil)

	hdr := map[string]string{"Authorization": "Bearer " + tok}
	code, h, page := a.do("GET", "/api/v1/audit-events?limit=1", nil, hdr)
	if code != 200 || h.Get("Cache-Control") != "no-store" {
		t.Fatal(code, page, h)
	}
	items := page["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["eventId"] != resp || page["nextCursor"] == nil {
		t.Fatal("filter after pagination", page)
	}
	first := items[0].(map[string]any)
	for _, key := range []string{"actorId", "issuer", "agentKey", "subject", "clientIp", "reason", "target"} {
		if _, exists := first[key]; exists {
			t.Fatal("sensitive metadata delivered", key, first)
		}
	}
	caps := first["capabilities"].(map[string]any)
	if caps["canReadMetadata"] != true || caps["canReadIdentities"] != false || caps["canReadResponsePayload"] != false {
		t.Fatal(caps)
	}
	_, _, second := a.do("GET", "/api/v1/audit-events?limit=1&cursor="+page["nextCursor"].(string), nil, hdr)
	next := second["items"].([]any)[0].(map[string]any)
	if next["eventId"] != req || next["capabilities"].(map[string]any)["canReadRequestPayload"] != true || second["nextCursor"] != nil {
		t.Fatal(second)
	}
	raw, _ := json.Marshal(page)
	if strings.Contains(string(raw), hidden) || strings.Contains(string(raw), "private-") || strings.Contains(string(raw), "private-message") {
		t.Fatal("list leaked hidden metadata/payload", string(raw))
	}
	for _, filter := range []string{"actorId=private-actor", "subject=private-subject", "agentKey=private-key"} {
		code, _, _ = a.do("GET", "/api/v1/audit-events?"+filter, nil, hdr)
		if code != 403 {
			t.Fatal("identity filter oracle", filter, code)
		}
	}
	_, _, groups := a.do("GET", "/api/v1/audit-events?groupBy=conversation", nil, hdr)
	group := groups["items"].([]any)[0].(map[string]any)
	if group["records"] != float64(2) {
		t.Fatal("group counts included hidden rows", group)
	}
	if _, exists := group["actorId"]; exists {
		t.Fatal("group identity leaked", group)
	}
	// An invisible outcome cannot change the reader's incomplete-operation view.
	const operation = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	if _, err := a.pool.Exec(ctx, `UPDATE audit_events SET operation_id=$2,request_id=$2,stage='admission',status='started' WHERE event_id=$1`, req, operation); err != nil {
		t.Fatal(err)
	}
	if _, err := a.pool.Exec(ctx, `UPDATE audit_events SET operation_id=$2,request_id=$2,stage='outcome' WHERE event_id=$1`, hidden, operation); err != nil {
		t.Fatal(err)
	}
	_, _, incomplete := a.do("GET", "/api/v1/audit-events?incomplete=true", nil, hdr)
	pending := incomplete["items"].([]any)
	if len(pending) != 1 || pending[0].(map[string]any)["eventId"] != req {
		t.Fatal("hidden outcome changed visible completeness", incomplete)
	}
	// Read-only permission for identities/details does not grant response bodies.
	if _, err := a.pool.Exec(ctx, `INSERT INTO authorization_role_permissions SELECT id,permission FROM authorization_roles CROSS JOIN (VALUES('audit.identities.read'),('audit.details.read')) AS p(permission) WHERE slug='payload-test'`); err != nil {
		t.Fatal(err)
	}
	_, _, allowed := a.do("GET", "/api/v1/audit-events?actorId=private-actor", nil, hdr)
	for _, row := range allowed["items"].([]any) {
		e := row.(map[string]any)
		if e["actorId"] != "private-actor" || e["reason"] != "private-reason" || e["capabilities"].(map[string]any)["canReadResponsePayload"] != false {
			t.Fatal("field permissions", e)
		}
	}
	controlPayloadRead(t, a, tok, resp, 404)
	if _, err := a.pool.Exec(ctx, `UPDATE authorization_assignments SET enabled=false`); err != nil {
		t.Fatal(err)
	}
	code, _, _ = a.do("GET", "/api/v1/audit-events", nil, hdr)
	if code != 403 {
		t.Fatal("metadata revoke not enforced", code)
	}
}

func TestAuditLegacyMetadataRequiresExplicitTenantGrant(t *testing.T) {
	a := setup(t)
	ctx := context.Background()
	tok, id := payloadReader(t, a)
	legacy := payloadEvent(t, a, "request")
	payloadGrant(t, a, id, []string{"audit.events.read"}, "tenant", nil, nil)

	hdr := map[string]string{"Authorization": "Bearer " + tok}
	_, _, page := a.do("GET", "/api/v1/audit-events", nil, hdr)
	for _, item := range page["items"].([]any) {
		if item.(map[string]any)["eventId"] == legacy {
			t.Fatal("legacy metadata granted implicitly")
		}
	}
	if _, err := a.pool.Exec(ctx, `INSERT INTO authorization_role_permissions SELECT id,'audit.legacy.events.read' FROM authorization_roles WHERE slug='payload-test'`); err != nil {
		t.Fatal(err)
	}
	_, _, page = a.do("GET", "/api/v1/audit-events", nil, hdr)
	found := false
	for _, item := range page["items"].([]any) {
		e := item.(map[string]any)
		if e["eventId"] == legacy {
			found = true
			if e["capabilities"].(map[string]any)["canReadPayload"] != false {
				t.Fatal("legacy metadata granted message")
			}
		}
	}
	if !found {
		t.Fatal("explicit legacy metadata missing")
	}
}
