package controlplane_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/store"
)

func payloadEvent(t *testing.T, a *api, direction string) string {
	t.Helper()
	ctx := context.Background()
	id, err := audit.NewID()
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := a.control.Store.Box.SealBound([]byte(`{"message":"private-message"}`), id, "audit.payload")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.pool.Exec(ctx, `INSERT INTO audit_events(event_id,method,status,virtual_server,scope,direction,payload_state) VALUES($1,'payload-fixture','ok','','virtual',$2,'captured')`, id, direction); err != nil {
		t.Fatal(err)
	}
	if _, err = a.pool.Exec(ctx, `INSERT INTO audit_payloads(event_id,ciphertext,expires_at) VALUES($1,$2,now()+interval '1 day')`, id, cipher); err != nil {
		t.Fatal(err)
	}
	return id
}
func payloadReader(t *testing.T, a *api) (string, string) {
	t.Helper()
	_, body := a.expect(201, "POST", "/api/v1/admin-tokens", map[string]any{"slug": "payload-investigator", "scope": "read"})
	var id string
	if err := a.pool.QueryRow(context.Background(), `SELECT id::text FROM admin_tokens WHERE slug='payload-investigator'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return body["token"].(string), id
}
func payloadGrant(t *testing.T, a *api, id string, permissions []string, scopeKind string, scopeID any, epoch any) {
	t.Helper()
	ctx := context.Background()
	var role string
	if err := a.pool.QueryRow(ctx, `INSERT INTO authorization_roles(slug,name) VALUES('payload-test','Payload Test') RETURNING id::text`).Scan(&role); err != nil {
		t.Fatal(err)
	}
	for _, p := range permissions {
		if _, err := a.pool.Exec(ctx, `INSERT INTO authorization_role_permissions(role_id,permission_id) VALUES($1,$2)`, role, p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.pool.Exec(ctx, `INSERT INTO authorization_assignments(role_id,principal_kind,principal_id,scope_kind,scope_id,epoch_id,capture_scopes) VALUES($1,'control_token',$2,$3,$4,$5,ARRAY['virtual'])`, role, id, scopeKind, scopeID, epoch); err != nil {
		t.Fatal(err)
	}
}

func TestPayloadAuthorizationRolloutAndLegacyDirections(t *testing.T) {
	a := setup(t)
	ctx := context.Background()
	tok, id := payloadReader(t, a)
	request := payloadEvent(t, a, "request")
	response := payloadEvent(t, a, "response")
	unknown := payloadEvent(t, a, "")
	hdr := map[string]string{"Authorization": "Bearer " + tok}
	path := func(id string) string { return "/api/v1/audit-events/" + id + "/payload" }
	payloadGrant(t, a, id, []string{"audit.events.read", "audit.legacy.events.read", "audit.legacy.payloads.read", "audit.payloads.request.read"}, "tenant", nil, nil)
	code, h, body := a.do("GET", path(request), nil, hdr)
	raw, _ := json.Marshal(body)
	if code != 200 || h.Get("Cache-Control") != "no-store" || !strings.Contains(string(raw), "private-message") {
		t.Fatal(code, h, body)
	}
	for _, eid := range []string{response, unknown, "00000000-0000-0000-0000-000000000000"} {
		code, h, body = a.do("GET", path(eid)+"?direction=request", nil, hdr)
		if code != 404 || h.Get("Cache-Control") != "no-store" || body["detail"] != "Audit event not found" {
			t.Fatal("payload direction/existence leaked", code, body)
		}
	}
	// Dynamic payload mode does not grant write access or audit-settings access.
	code, _, _ = a.do("POST", "/api/v1/upstreams", map[string]any{}, hdr)
	if code != 403 {
		t.Fatal("unrelated API broadened", code)
	}
	code, _, _ = a.do("GET", "/api/v1/audit-settings", nil, hdr)
	if code != 403 {
		t.Fatal("settings broadened", code)
	}
	a.expect(200, "GET", path(unknown), nil) // platform admin compatibility
	if _, err := a.pool.Exec(ctx, `UPDATE authorization_assignments SET enabled=false`); err != nil {
		t.Fatal(err)
	}
	code, _, _ = a.do("GET", path(request), nil, hdr)
	if code != 404 {
		t.Fatal("revocation stale", code)
	}
	// Even with read capture enabled, viewing a message must not store it again.
	s := settingsFor(t, a)
	s.Scopes["reads"] = audit.Capture{Request: true, Response: true}
	a.expect(200, "PUT", "/api/v1/audit-settings", s)
	a.expect(200, "GET", path(request), nil)
	var count int
	if err := a.pool.QueryRow(ctx, `SELECT count(*) FROM audit_payloads p JOIN audit_events e USING(event_id) WHERE e.action IN ('GET /api/v1/audit-events/{event}/payload','audit.payload.access')`).Scan(&count); err != nil || count != 0 {
		t.Fatal("payload read recaptured", count, err)
	}
}

func TestPayloadAuthorizationBeforeCiphertextDecryption(t *testing.T) {
	a := setup(t)
	ctx := context.Background()
	tok, id := payloadReader(t, a)
	eid := payloadEvent(t, a, "response")
	payloadGrant(t, a, id, []string{"audit.events.read", "audit.legacy.events.read", "audit.legacy.payloads.read", "audit.payloads.request.read"}, "tenant", nil, nil)
	if _, err := a.pool.Exec(ctx, `UPDATE audit_payloads SET ciphertext='invalid'::bytea WHERE event_id=$1`, eid); err != nil {
		t.Fatal(err)
	}

	hdr := map[string]string{"Authorization": "Bearer " + tok}
	path := "/api/v1/audit-events/" + eid + "/payload"
	code, _, _ := a.do("GET", path, nil, hdr)
	if code != 404 {
		t.Fatal("decryption reached before authorization", code)
	}
	if _, err := a.pool.Exec(ctx, `INSERT INTO authorization_role_permissions SELECT id,'audit.payloads.response.read' FROM authorization_roles WHERE slug='payload-test'`); err != nil {
		t.Fatal(err)
	}
	code, _, _ = a.do("GET", path, nil, hdr)
	if code != 503 {
		t.Fatal("authorized corrupt ciphertext did not fail", code)
	}
}

func TestPayloadAuthorizationHistoricalEpochAndConsumerDomain(t *testing.T) {
	a := setup(t)
	ctx := context.Background()
	tok, id := payloadReader(t, a)
	old := payloadEvent(t, a, "response")
	newer := payloadEvent(t, a, "response")
	legacy := payloadEvent(t, a, "response")
	const resource = "11111111-1111-1111-1111-111111111111"
	const oldTeam = "22222222-2222-2222-2222-222222222222"
	const newTeam = "33333333-3333-3333-3333-333333333333"
	addContext := func(event, owner string, start int, close bool) string {
		t.Helper()
		var epoch string
		if err := a.pool.QueryRow(ctx, `INSERT INTO audit_resource_epochs(resource_kind,resource_id,owner_team_id,start_revision,end_revision) VALUES('virtual_server',$1,$2,$3::bigint,CASE WHEN $4::boolean THEN $3::bigint+1 ELSE NULL::bigint END) RETURNING epoch_id::text`, resource, owner, start, close).Scan(&epoch); err != nil {
			t.Fatal(err)
		}
		if _, err := a.pool.Exec(ctx, `INSERT INTO audit_access_contexts(event_id,classification,origin,operation_config_revision,provenance_state) VALUES($1,'resource','virtual',$2,'verified')`, event, start); err != nil {
			t.Fatal(err)
		}
		if _, err := a.pool.Exec(ctx, `INSERT INTO audit_access_domains(event_id,domain_kind,resource_kind,resource_id,owner_team_id,epoch_id) VALUES($1,'consumer','virtual_server',$2,$3,$4)`, event, resource, owner, epoch); err != nil {
			t.Fatal(err)
		}
		return epoch
	}
	epoch := addContext(old, oldTeam, 1, true)
	addContext(newer, newTeam, 2, false)
	var providerEpoch string
	if err := a.pool.QueryRow(ctx, `INSERT INTO audit_resource_epochs(resource_kind,resource_id,owner_team_id,start_revision) VALUES('upstream','44444444-4444-4444-4444-444444444444',$1,1) RETURNING epoch_id::text`, newTeam).Scan(&providerEpoch); err != nil {
		t.Fatal(err)
	}
	if _, err := a.pool.Exec(ctx, `INSERT INTO audit_access_domains(event_id,domain_kind,resource_kind,resource_id,owner_team_id,epoch_id) VALUES($1,'provider','upstream','44444444-4444-4444-4444-444444444444',$2,$3)`, old, newTeam, providerEpoch); err != nil {
		t.Fatal(err)
	}

	payloadGrant(t, a, id, []string{"audit.events.read", "audit.payloads.response.read"}, "virtual_server", resource, epoch)

	hdr := map[string]string{"Authorization": "Bearer " + tok}
	for _, c := range []struct {
		id   string
		code int
	}{{old, 200}, {newer, 404}, {legacy, 404}} {
		code, _, _ := a.do("GET", "/api/v1/audit-events/"+c.id+"/payload", nil, hdr)
		if code != c.code {
			t.Fatal(c, code)
		}
	}
	// An upstream provider grant cannot replace a consumer grant.
	if _, err := a.pool.Exec(ctx, `UPDATE authorization_assignments SET scope_kind='team',scope_id=$1,epoch_id=NULL`, newTeam); err != nil {
		t.Fatal(err)
	}
	code, _, _ := a.do("GET", "/api/v1/audit-events/"+old+"/payload", nil, hdr)
	if code != 404 {
		t.Fatal("new owner inherited old message", code)
	}
}

func TestPayloadAuthorizationTenantBoundaryAndPolicyFailure(t *testing.T) {
	a := setup(t)
	ctx := context.Background()
	eid := payloadEvent(t, a, "request")

	if _, err := a.pool.Exec(ctx, `UPDATE audit_events SET tenant_id='another-tenant' WHERE event_id=$1`, eid); err != nil {
		t.Fatal(err)
	}
	// Even bootstrap compatibility cannot cross the deployment tenant.
	a.expect(404, "GET", "/api/v1/audit-events/"+eid+"/payload", nil)
	if _, err := a.pool.Exec(ctx, `ALTER TABLE authorization_state RENAME TO unavailable_authorization_state`); err != nil {
		t.Fatal(err)
	}
	code, h, _ := a.do("GET", "/api/v1/audit-events/"+eid+"/payload", nil, nil)
	if code != 503 || h.Get("Cache-Control") != "no-store" {
		t.Fatal("policy failure did not close access", code, h)
	}
}

func TestPayloadAPIUsesProducerSnapshotAcrossProjectTransfer(t *testing.T) {
	a := setup(t)
	ctx := context.Background()
	for _, team := range []string{"old-team", "new-team"} {
		a.expect(200, "POST", "/api/v1/teams", map[string]any{"slug": team, "name": team, "issuer": "https://idp", "group": team})
	}
	a.expect(200, "POST", "/api/v1/projects", map[string]any{"slug": "product", "name": "Product", "ownerTeam": "old-team"})
	a.expect(201, "POST", "/api/v1/virtual-servers", map[string]any{"slug": "chat", "name": "Chat", "project": "product"})
	old, err := store.LoadSnapshot(ctx, a.pool, a.control.Store.Box)
	if err != nil {
		t.Fatal(err)
	}
	writerCtx, cancel := context.WithCancel(ctx)
	runtime := audit.NewRuntime(writerCtx, a.pool, a.control.Store.Box, a.control.Log, "strict")
	defer func() { cancel(); <-runtime.Done() }()
	settings := audit.DefaultSettings()
	settings.Scopes["virtual"] = audit.Capture{Request: true, Response: true}
	runtime.SetSettings(settings)
	event, err := runtime.Admit(ctx, audit.Event{Access: old.ConsumerAudit(old.VirtualServers["chat"]), Revision: old.Revision, Method: "snapshot-transfer", Scope: "virtual", RawPayload: []byte(`{"message":"old request"}`)})
	if err != nil {
		t.Fatal(err)
	}
	var projectVersion int64
	if err = a.pool.QueryRow(ctx, `SELECT version FROM projects WHERE slug='product'`).Scan(&projectVersion); err != nil {
		t.Fatal(err)
	}
	a.expect(200, "POST", "/api/v1/projects", map[string]any{"slug": "product", "name": "Product", "ownerTeam": "new-team", "version": projectVersion})
	event.Status = "ok"
	event.RawPayload = []byte(`{"message":"old response after transfer"}`)
	if err = runtime.Complete(ctx, event); err != nil {
		t.Fatal(err)
	}
	fresh, err := store.LoadSnapshot(ctx, a.pool, a.control.Store.Box)
	if err != nil {
		t.Fatal(err)
	}
	next, err := runtime.Admit(ctx, audit.Event{Access: fresh.ConsumerAudit(fresh.VirtualServers["chat"]), Revision: fresh.Revision, Method: "snapshot-transfer", Scope: "virtual", RawPayload: []byte(`{"message":"new request"}`)})
	if err != nil {
		t.Fatal(err)
	}
	tok, tokenID := payloadReader(t, a)
	oldOwner := old.VirtualServers["chat"].AuditDomain.OwnerTeamID
	newOwner := fresh.VirtualServers["chat"].AuditDomain.OwnerTeamID
	payloadGrant(t, a, tokenID, []string{"audit.events.read", "audit.payloads.request.read", "audit.payloads.response.read"}, "team", oldOwner, nil)

	hdr := map[string]string{"Authorization": "Bearer " + tok}
	read := func(id string, want int) {
		t.Helper()
		code, _, _ := a.do("GET", "/api/v1/audit-events/"+id+"/payload", nil, hdr)
		if code != want {
			t.Fatalf("event %s got %d want %d", id, code, want)
		}
	}
	read(event.EventID, 200)
	read(audit.OutcomeID(event.OperationID), 200)
	read(next.EventID, 404)
	if _, err = a.pool.Exec(ctx, `UPDATE authorization_assignments SET scope_id=$1`, newOwner); err != nil {
		t.Fatal(err)
	}
	read(event.EventID, 404)
	read(audit.OutcomeID(event.OperationID), 404)
	read(next.EventID, 200)
}
