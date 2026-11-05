package controlplane_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/controlplane"
	"github.com/bishoku/vmcp/core/internal/oidc"
	"github.com/bishoku/vmcp/core/internal/testidp"
)

func mappingAPI(t *testing.T) (*api, *testidp.IdP) {
	t.Helper()
	idp := testidp.New(t)
	provider, err := oidc.New(context.Background(), oidc.Config{Issuer: idp.URL(), ClientID: "control-client", RedirectURL: "http://localhost/auth/callback", ViewerGroups: []string{"viewers"}, SessionKey: []byte(strings.Repeat("k", 32)), HTTPClient: idp.Srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return setup(t, func(s *controlplane.Server) { s.OIDC = provider }), idp
}
func mappingInput(issuer string, enabled bool) map[string]any {
	return map[string]any{"issuer": issuer, "principalKind": "oidc", "enabled": enabled, "conditions": []map[string]any{{"path": "groups", "operator": "array_contains", "value": "reviewers"}, {"path": "scope", "operator": "scope_contains", "value": "config:read"}, {"path": "organization.department", "operator": "equals", "value": "security"}}}
}
func TestAuthorizationMappingCRUDAndTrustedIssuer(t *testing.T) {
	a, idp := mappingAPI(t)
	read, _ := payloadReader(t, a)
	for _, path := range []string{"/api/v1/identity-mappings", "/api/v1/access/status"} {
		code, _, _ := a.do("GET", path, nil, map[string]string{"Authorization": "Bearer " + read})
		if code != 403 {
			t.Fatal(path, code)
		}
	}
	for _, path := range []string{"/api/v1/identity-mappings", "/api/v1/access/evaluate"} {
		code, _, _ := a.do("POST", path, mappingInput(idp.URL(), true), map[string]string{"Authorization": "Bearer " + read})
		if code != 403 {
			t.Fatal(path, code)
		}
	}
	in := mappingInput(idp.URL(), true)
	out := assignmentCall(t, a, 201, "POST", "/api/v1/identity-mappings", in, "")
	id := out["id"].(string)
	path := "/api/v1/identity-mappings/" + id
	if out["version"] != float64(1) {
		t.Fatal(out)
	}
	assignmentCall(t, a, 200, "GET", path, nil, "")
	assignmentCall(t, a, 428, "PUT", path, in, "")
	assignmentCall(t, a, 412, "PUT", path, in, "\"9\"")
	in["enabled"] = false
	out = assignmentCall(t, a, 200, "PUT", path, in, "\"1\"")
	if out["version"] != float64(2) {
		t.Fatal(out)
	}
	assignmentCall(t, a, 412, "DELETE", path, nil, "\"1\"")
	for _, issuer := range []string{idp.URL() + "/", idp.URL() + "/other", "https://foreign.invalid"} {
		assignmentCall(t, a, 422, "POST", "/api/v1/identity-mappings", mappingInput(issuer, true), "")
	}
	tests := []struct {
		name   string
		change func(map[string]any)
	}{
		{"wrong kind", func(v map[string]any) { v["principalKind"] = "control_token" }},
		{"empty conditions", func(v map[string]any) { v["conditions"] = []any{} }},
		{"wrong operator", func(v map[string]any) {
			v["conditions"] = []map[string]any{{"path": "scope", "operator": "contains", "value": "read"}}
		}},
		{"multi token", func(v map[string]any) {
			v["conditions"] = []map[string]any{{"path": "scope", "operator": "scope_contains", "value": "read write"}}
		}},
		{"wrong type", func(v map[string]any) {
			v["conditions"] = []map[string]any{{"path": "groups", "operator": "equals", "value": 42}}
		}},
		{"blank path", func(v map[string]any) {
			v["conditions"] = []map[string]any{{"path": " ", "operator": "equals", "value": "read"}}
		}},
		{"unknown field", func(v map[string]any) { v["audience"] = "new-trust-root" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := mappingInput(idp.URL(), true)
			tt.change(v)
			assignmentCall(t, a, 422, "POST", "/api/v1/identity-mappings", v, "")
		})
	}
	// Untrusted drafts can be listed but cannot be enabled, even after creation.
	draft := mappingInput("https://foreign.invalid", false)
	d := assignmentCall(t, a, 201, "POST", "/api/v1/identity-mappings", draft, "")
	draft["enabled"] = true
	assignmentCall(t, a, 422, "PUT", "/api/v1/identity-mappings/"+d["id"].(string), draft, "\"1\"")
	page := assignmentCall(t, a, 200, "GET", "/api/v1/identity-mappings?enabled=false&limit=1", nil, "")
	cursor, ok := page["nextCursor"].(string)
	if !ok {
		t.Fatal(page)
	}
	next := assignmentCall(t, a, 200, "GET", "/api/v1/identity-mappings?enabled=false&limit=1&cursor="+cursor, nil, "")
	if next["nextCursor"] != nil {
		t.Fatal(next)
	}
	assignmentCall(t, a, 204, "DELETE", path, nil, "\"2\"")
	assignmentCall(t, a, 404, "GET", path, nil, "")
	// Without an OIDC provider, only disabled mapping drafts can be saved.
	b := setup(t)
	assignmentCall(t, b, 422, "POST", "/api/v1/identity-mappings", mappingInput(idp.URL(), true), "")
	assignmentCall(t, b, 201, "POST", "/api/v1/identity-mappings", mappingInput(idp.URL(), false), "")
}

func TestAuthorizationSimulationTypedClaimsAndNoAuthentication(t *testing.T) {
	a, idp := mappingAPI(t)
	mapping := assignmentCall(t, a, 201, "POST", "/api/v1/identity-mappings", mappingInput(idp.URL(), true), "")
	role := assignmentRole(t, a, "mapped-reader", "upstreams.read")
	assignment := map[string]any{"roleId": role, "mappingId": mapping["id"], "scope": map[string]any{"kind": "tenant"}, "enabled": true}
	grant := assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", assignment, "")
	samples := []struct {
		name    string
		claims  map[string]any
		allowed bool
	}{
		{"typed match", map[string]any{"groups": []string{"reviewers"}, "scope": "other config:read"}, true},
		{"scope boundary", map[string]any{"groups": []string{"reviewers"}, "scope": "config:read:all"}, false},
		{"array not string", map[string]any{"groups": "reviewers", "scope": "config:read"}, false},
		{"scope not array", map[string]any{"groups": []string{"reviewers"}, "scope": []string{"config:read"}}, false},
		{"missing claim", map[string]any{"scope": "config:read"}, false},
		{"equals wrong type", map[string]any{"groups": []string{"reviewers"}, "scope": "config:read", "organization.department": 42}, false},
		{"exact key before dotted path", map[string]any{"groups": []string{"reviewers"}, "scope": "config:read", "organization.department": "different"}, false},
	}
	var rev int64
	if err := a.pool.QueryRow(context.Background(), `SELECT revision FROM authorization_state`).Scan(&rev); err != nil {
		t.Fatal(err)
	}
	for _, tt := range samples {
		tt.claims["organization"] = map[string]any{"department": "security"}
		t.Run(tt.name, func(t *testing.T) {
			in := map[string]any{"principal": map[string]any{"kind": "oidc", "issuer": idp.URL(), "id": "sample-user"}, "claims": tt.claims, "permission": "upstreams.read", "resource": map[string]any{"kind": "tenant"}}
			status, h, result := a.do("POST", "/api/v1/access/evaluate", in, nil)
			if status != 200 || h.Get("Set-Cookie") != "" || result["simulationOnly"] != true || result["authenticationValidated"] != false || result["legacyCompatibilityIncluded"] != nil || result["decision"].(map[string]any)["allowed"] != tt.allowed {
				t.Fatal(status, h, result)
			}
			if tt.allowed && len(result["matchedMappingIds"].([]any)) != 1 {
				t.Fatal(result)
			}
		})
	}
	var after int64
	if err := a.pool.QueryRow(context.Background(), `SELECT revision FROM authorization_state`).Scan(&after); err != nil || rev != after {
		t.Fatal("simulation mutated policy", rev, after, err)
	}
	// A perfectly signed token for the configured issuer is still not a control API bearer.
	signed := idp.Mint(t, testidp.Token{Audience: []string{"control-client"}, Claims: map[string]any{"groups": []string{"reviewers"}, "scope": "config:read"}})
	status, _, _ := a.do("GET", "/api/v1/upstreams", nil, map[string]string{"Authorization": "Bearer " + signed})
	if status != 401 {
		t.Fatal("external bearer accepted", status)
	}
	// Used mappings cannot be deleted. Disabling an ordinary mapping changes only simulation.
	assignmentCall(t, a, 409, "DELETE", "/api/v1/identity-mappings/"+mapping["id"].(string), nil, "\"1\"")
	in := mappingInput(idp.URL(), false)
	assignmentCall(t, a, 200, "PUT", "/api/v1/identity-mappings/"+mapping["id"].(string), in, "\"1\"")
	sample := map[string]any{"principal": map[string]any{"kind": "oidc", "issuer": idp.URL(), "id": "sample-user"}, "claims": samples[0].claims, "permission": "upstreams.read", "resource": map[string]any{"kind": "tenant"}}
	result := assignmentCall(t, a, 200, "POST", "/api/v1/access/evaluate", sample, "")
	if result["decision"].(map[string]any)["allowed"] != false {
		t.Fatal(result)
	}
	// Changing an issuer underneath an enabled assignment is forbidden even for a disabled draft.
	in["issuer"] = "https://foreign.invalid"
	assignmentCall(t, a, 409, "PUT", "/api/v1/identity-mappings/"+mapping["id"].(string), in, "\"2\"")
	assignment["enabled"] = false
	assignmentCall(t, a, 200, "PUT", "/api/v1/role-assignments/"+grant["id"].(string), assignment, "\"1\"")
	assignmentCall(t, a, 409, "DELETE", "/api/v1/identity-mappings/"+mapping["id"].(string), nil, "\"2\"")
	assignmentCall(t, a, 204, "DELETE", "/api/v1/role-assignments/"+grant["id"].(string), nil, "\"2\"")
	assignmentCall(t, a, 204, "DELETE", "/api/v1/identity-mappings/"+mapping["id"].(string), nil, "\"2\"")
}

func TestAuthorizationOperatorMappingCanBeDisabledAndReconditioned(t *testing.T) {
	a, idp := mappingAPI(t)
	in := mappingInput(idp.URL(), true)
	m := assignmentCall(t, a, 201, "POST", "/api/v1/identity-mappings", in, "")
	_, _, role := a.do("GET", "/api/v1/roles/platform-operator", nil, nil)
	assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", map[string]any{"roleId": role["id"], "mappingId": m["id"], "scope": map[string]any{"kind": "tenant"}, "captureScopes": []string{"control"}, "enabled": true}, "")
	path := "/api/v1/identity-mappings/" + m["id"].(string)
	in["enabled"] = false
	assignmentCall(t, a, 200, "PUT", path, in, "\"1\"")
	in["enabled"] = true
	in["conditions"] = []map[string]any{{"path": "groups", "operator": "array_contains", "value": "nobody"}}
	assignmentCall(t, a, 200, "PUT", path, in, "\"2\"")
	// References and trusted-issuer boundaries still apply.
	assignmentCall(t, a, 409, "DELETE", path, nil, "\"3\"")
	in["issuer"] = "https://untrusted.example"
	in["enabled"] = false
	assignmentCall(t, a, 409, "PUT", path, in, "\"3\"")
}

func TestAuthorizationSimulationPayloadPrivacyAndAccessStatus(t *testing.T) {
	a, _, _ := controlAuditFixture(t)
	token, id := payloadReader(t, a)
	settings := settingsFor(t, a)
	settings.Scopes["control"] = audit.Capture{Request: true, Response: true}
	settings.Scopes["reads"] = audit.Capture{Request: true, Response: true}
	a.expect(200, "PUT", "/api/v1/audit-settings", settings)
	_, h, _ := a.do("PATCH", "/api/v1/upstreams/crm", map[string]any{"description": "private-sample"}, nil)
	request := awaitControlEvent(t, a, h.Get("X-Request-Id"), "admission")
	response := awaitControlEvent(t, a, h.Get("X-Request-Id"), "outcome")
	role := assignmentRole(t, a, "simulated-reader", "audit.events.read", "audit.payloads.request.read")
	assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", assignmentInput(role, id, map[string]any{"kind": "tenant"}), "")

	in := map[string]any{"principal": map[string]any{"kind": "control_token", "id": id}, "permission": "audit.payloads.request.read", "auditEventId": request}
	code, headers, result := a.do("POST", "/api/v1/access/evaluate", in, nil)
	if code != 200 || result["decision"].(map[string]any)["allowed"] != true {
		t.Fatal(code, result)
	}
	for _, stage := range []string{"admission", "outcome"} {
		event := awaitControlEvent(t, a, headers.Get("X-Request-Id"), stage)
		var n int
		if err := a.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_payloads WHERE event_id=$1`, event).Scan(&n); err != nil || n != 0 {
			t.Fatal("simulation content captured", n, err)
		}
	}
	in["auditEventId"] = response
	result = assignmentCall(t, a, 200, "POST", "/api/v1/access/evaluate", in, "")
	if result["decision"].(map[string]any)["allowed"] != false {
		t.Fatal("wrong direction", result)
	}
	var revision int64
	if err := a.pool.QueryRow(context.Background(), `SELECT revision FROM authorization_state`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	status := assignmentCall(t, a, 200, "GET", "/api/v1/access/status", nil, "")
	if status["authorizationRevision"] != float64(revision) || status["mode"] != nil || status["auditMode"] != nil || status["configurationEnforcementReady"] != nil || status["configurationEnforcement"] != nil || status["claimMappings"] != "simulation-only" || status["externalOAuthBearer"] != false {
		t.Fatal(status)
	}
	if status["schemaVersion"].(float64) < 38 {
		t.Fatal(status)
	}
	// Audit mode and existing direct-token behavior survive simulation/status reads.
	controlPayloadRead(t, a, token, request, 200)
	controlPayloadRead(t, a, token, response, 404)
	var after int64
	if err := a.pool.QueryRow(context.Background(), `SELECT revision FROM authorization_state`).Scan(&after); err != nil || revision != after {
		t.Fatal("status changed policy", revision, after, err)
	}
	req, err := http.NewRequest("POST", a.srv.URL+"/api/v1/access/evaluate", strings.NewReader(`{"token":"secret-token"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal(res.StatusCode)
	}
}

func TestAuthorizationSimulationRejectsUntrustedAndSpoofedContext(t *testing.T) {
	a, idp := mappingAPI(t)
	good := func() map[string]any {
		return map[string]any{"principal": map[string]any{"kind": "oidc", "issuer": idp.URL(), "id": "sample-user"}, "permission": "upstreams.read", "resource": map[string]any{"kind": "tenant"}}
	}
	for _, edit := range []func(map[string]any){
		func(in map[string]any) {
			in["principal"] = map[string]any{"kind": "oidc", "issuer": idp.URL() + "/", "id": "sample-user"}
		},
		func(in map[string]any) { in["claims"] = map[string]any{"iss": "https://foreign.invalid"} },
		func(in map[string]any) { in["claims"] = map[string]any{"sub": "another-user"} },
		func(in map[string]any) { in["permission"] = "invented.action" },
		func(in map[string]any) { in["resource"] = map[string]any{"kind": "upstream", "id": "bad"} },
		func(in map[string]any) { in["resource"] = map[string]any{"kind": "tenant", "tenant": "spoofed"} },
		func(in map[string]any) { in["resource"] = map[string]any{"kind": "tenant", "ownerTeam": "spoofed"} },
		func(in map[string]any) { in["resource"] = map[string]any{"kind": "tenant", "epoch": "spoofed"} },
		func(in map[string]any) { in["auditEventId"] = "00000000-0000-0000-0000-000000000001" },
		func(in map[string]any) { delete(in, "resource") },
		func(in map[string]any) { in["permission"] = "audit.payloads.request.read" },
		func(in map[string]any) { in["token"] = "raw-token-not-accepted" },
	} {
		in := good()
		edit(in)
		assignmentCall(t, a, 422, "POST", "/api/v1/access/evaluate", in, "")
	}
	in := good()
	in["resource"] = map[string]any{"kind": "upstream", "id": "00000000-0000-0000-0000-000000000001"}
	assignmentCall(t, a, 404, "POST", "/api/v1/access/evaluate", in, "")
	delete(in, "resource")
	in["auditEventId"] = "00000000-0000-0000-0000-000000000001"
	assignmentCall(t, a, 404, "POST", "/api/v1/access/evaluate", in, "")
	// Only current configured issuer is trusted after a provider configuration change.
	original := mappingInput(idp.URL(), true)
	m := assignmentCall(t, a, 201, "POST", "/api/v1/identity-mappings", original, "")
	other := testidp.New(t)
	provider, err := oidc.New(context.Background(), oidc.Config{Issuer: other.URL(), ClientID: "client", RedirectURL: "http://localhost/callback", ViewerGroups: []string{"viewers"}, SessionKey: []byte(strings.Repeat("k", 32)), HTTPClient: other.Srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	// Server config is immutable during normal serving; use a fresh API with the new provider.
	b := setup(t, func(s *controlplane.Server) { s.OIDC = provider })
	assignmentCall(t, b, 422, "POST", "/api/v1/identity-mappings", original, "")
	// The old draft can still be disabled with its old issuer when no active assignment refers to it.
	original["enabled"] = false
	assignmentCall(t, a, 200, "PUT", "/api/v1/identity-mappings/"+m["id"].(string), original, "\"1\"")
}

func TestAuthorizationOperatorMappingsCanBothBeDisabled(t *testing.T) {
	a, idp := mappingAPI(t)
	_, _, role := a.do("GET", "/api/v1/roles/platform-operator", nil, nil)
	mappings := []string{}
	for i := 0; i < 2; i++ {
		m := assignmentCall(t, a, 201, "POST", "/api/v1/identity-mappings", mappingInput(idp.URL(), true), "")
		mappings = append(mappings, m["id"].(string))
		assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", map[string]any{"roleId": role["id"], "mappingId": m["id"], "scope": map[string]any{"kind": "tenant"}, "captureScopes": []string{"control"}, "enabled": true}, "")
	}
	results := make(chan int, 2)
	for _, id := range mappings {
		go func(id string) {
			code, _, _ := a.do("PUT", "/api/v1/identity-mappings/"+id, mappingInput(idp.URL(), false), map[string]string{"If-Match": "\"1\""})
			results <- code
		}(id)
	}
	one, two := <-results, <-results
	if one != 200 || two != 200 {
		t.Fatal("independent operator mappings must both be revocable", one, two)
	}
}

func TestAuthorizationClaimSamplesAreNotAuditedAndModesStayLegacy(t *testing.T) {
	a, idp := mappingAPI(t)
	settings := settingsFor(t, a)
	settings.Scopes["control"] = audit.Capture{Request: true, Response: true}
	settings.Scopes["reads"] = audit.Capture{Request: true, Response: true}
	a.expect(200, "PUT", "/api/v1/audit-settings", settings)
	in := mappingInput(idp.URL(), true)
	in["conditions"] = []map[string]any{{"path": "sensitive", "operator": "equals", "value": "sample-private-marker"}}
	code, h, _ := a.do("POST", "/api/v1/identity-mappings", in, nil)
	if code != 201 {
		t.Fatal(code)
	}
	check := func(headers http.Header) {
		t.Helper()
		for _, stage := range []string{"admission", "outcome"} {
			event := awaitControlEvent(t, a, headers.Get("X-Request-Id"), stage)
			var n int
			if err := a.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_payloads WHERE event_id=$1`, event).Scan(&n); err != nil || n != 0 {
				t.Fatal("claim data captured", n, err)
			}
		}
	}
	check(h)
	var before int64
	if err := a.pool.QueryRow(context.Background(), `SELECT revision FROM authorization_state`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	code, h, out := a.do("POST", "/api/v1/access/evaluate", map[string]any{"principal": map[string]any{"kind": "oidc", "issuer": idp.URL(), "id": "sample-sensitive-subject"}, "claims": map[string]any{"sensitive": "sample-private-marker"}, "permission": "upstreams.read", "resource": map[string]any{"kind": "tenant"}}, nil)
	if code != 200 {
		t.Fatal(code, out)
	}
	check(h)
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "sample-private-marker") || strings.Contains(string(raw), "sample-sensitive-subject") {
		t.Fatal("sample echoed", string(raw))
	}
	var leaked int
	if err := a.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_events e WHERE row_to_json(e)::text LIKE '%sample-private-marker%' OR row_to_json(e)::text LIKE '%sample-sensitive-subject%'`).Scan(&leaked); err != nil || leaked != 0 {
		t.Fatal("sample in audit metadata", leaked, err)
	}
	status := assignmentCall(t, a, 200, "GET", "/api/v1/access/status", nil, "")
	if status["mode"] != nil || status["auditMode"] != nil || status["authorizationRevision"] != float64(before) {
		t.Fatal("inspection activated modes", status)
	}
	// Unassigned tokens never inherit configuration permissions.
	token, _ := payloadReader(t, a)
	code, _, _ = a.do("GET", "/api/v1/upstreams", nil, map[string]string{"Authorization": "Bearer " + token})
	if code != 403 {
		t.Fatal("unassigned token gained read access", code)
	}
	code, _, _ = a.do("POST", "/api/v1/upstreams", map[string]any{"slug": "forbidden"}, map[string]string{"Authorization": "Bearer " + token})
	if code != 403 {
		t.Fatal("legacy write changed", code)
	}
}

func TestAuthorizationStatusDiagnosticsAndLegacyNoBackfill(t *testing.T) {
	a, _, _ := controlAuditFixture(t)
	_, token := payloadReader(t, a)
	ctx := context.Background()
	var resource string
	if err := a.pool.QueryRow(ctx, `SELECT id::text FROM upstreams WHERE slug='crm'`).Scan(&resource); err != nil {
		t.Fatal(err)
	}
	role := assignmentRole(t, a, "legacy-status-auditor", "audit.events.read", "audit.payloads.request.read")
	legacy := payloadEvent(t, a, "request")
	in := assignmentInput(role, token, map[string]any{"kind": "tenant"})
	assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", in, "")
	sim := map[string]any{"principal": in["principal"], "permission": "audit.payloads.request.read", "auditEventId": legacy}
	out := assignmentCall(t, a, 200, "POST", "/api/v1/access/evaluate", sim, "")
	if out["decision"].(map[string]any)["allowed"] != false {
		t.Fatal("legacy payload granted without legacy permissions", out)
	}
	assignmentCall(t, a, 200, "PUT", "/api/v1/roles/legacy-status-auditor", map[string]any{"name": "Legacy", "permissions": []string{"audit.events.read", "audit.legacy.events.read", "audit.legacy.payloads.read", "audit.payloads.request.read"}}, "\"1\"")
	// Fixture capture scope is virtual; explicitly include it in the assignment (already present).
	out = assignmentCall(t, a, 200, "POST", "/api/v1/access/evaluate", sim, "")
	if out["decision"].(map[string]any)["allowed"] != true {
		t.Fatal(out)
	}
	// Model an old/out-of-band configuration which API validation would now reject.
	var mapping string
	if err := a.pool.QueryRow(ctx, `INSERT INTO authorization_mappings(issuer,principal_kind,conditions) VALUES('https://foreign.invalid','oidc','[{"path":"scope","operator":"scope_contains","value":"read"}]') RETURNING id::text`).Scan(&mapping); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`INSERT INTO authorization_assignments(role_id,mapping_id,scope_kind) VALUES($1::uuid,$2::uuid,'tenant')`,
		`INSERT INTO authorization_assignments(role_id,principal_kind,principal_id,scope_kind,scope_id) VALUES($1::uuid,'control_token','missing-token','upstream',$2::uuid)`,
	} {
		arg := mapping
		if strings.Contains(sql, "scope_id") {
			arg = resource
		}
		if _, err := a.pool.Exec(ctx, sql, role, arg); err != nil {
			t.Fatal(err)
		}
	}
	status := assignmentCall(t, a, 200, "GET", "/api/v1/access/status", nil, "")
	counts := status["counts"].(map[string]any)
	for _, name := range []string{"untrustedEnabledMappings", "untrustedEnabledOIDCAssignments", "inactiveTokenAssignments", "unpinnedAuditResourceAssignments", "legacyAuditEvents"} {
		if counts[name].(float64) < 1 {
			t.Fatal(name, status)
		}
	}
	var contexts int
	if err := a.pool.QueryRow(ctx, `SELECT count(*) FROM audit_access_contexts WHERE event_id=$1`, legacy).Scan(&contexts); err != nil || contexts != 0 {
		t.Fatal("legacy context backfilled", contexts, err)
	}
}
