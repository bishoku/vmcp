package controlplane_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/authz"
	"github.com/bishoku/vmcp/core/internal/store"
)

func assignmentCall(t *testing.T, a *api, want int, method, path string, body any, etag string) map[string]any {
	t.Helper()
	headers := map[string]string{}
	if etag != "" {
		headers["If-Match"] = etag
	}
	code, h, out := a.do(method, path, body, headers)
	if code != want {
		t.Fatalf("%s %s: want %d got %d: %v", method, path, want, code, out)
	}
	if code == 200 || code == 201 {
		if h.Get("Cache-Control") != "no-store" {
			t.Fatal("missing no-store")
		}
	}
	return out
}
func assignmentRole(t *testing.T, a *api, slug string, permissions ...string) string {
	t.Helper()
	out := assignmentCall(t, a, 201, "POST", "/api/v1/roles", map[string]any{"slug": slug, "name": slug, "permissions": permissions}, "")
	return out["id"].(string)
}
func assignmentInput(role, token string, scope map[string]any) map[string]any {
	return map[string]any{"roleId": role, "principal": map[string]any{"kind": "control_token", "id": token}, "scope": scope, "captureScopes": []string{"control", "virtual"}, "enabled": true}
}
func TestAuthorizationAssignmentCRUDAndValidation(t *testing.T) {
	a := setup(t)
	read, token := payloadReader(t, a)
	role := assignmentRole(t, a, "request-investigator", "audit.events.read", "audit.payloads.request.read")
	input := assignmentInput(role, token, map[string]any{"kind": "tenant"})
	for _, method := range []string{"GET", "POST"} {
		code, _, _ := a.do(method, "/api/v1/role-assignments", input, map[string]string{"Authorization": "Bearer " + read})
		if code != 403 {
			t.Fatal("nonadmin", method, code)
		}
	}
	out := assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", input, "")
	id := out["id"].(string)
	path := "/api/v1/role-assignments/" + id
	if out["version"] != float64(1) {
		t.Fatal(out)
	}
	assignmentCall(t, a, 200, "GET", path, nil, "")
	assignmentCall(t, a, 428, "PUT", path, input, "")
	assignmentCall(t, a, 412, "PUT", path, input, "\"9\"")
	input["enabled"] = false
	out = assignmentCall(t, a, 200, "PUT", path, input, "\"1\"")
	if out["version"] != float64(2) {
		t.Fatal(out)
	}
	list := assignmentCall(t, a, 200, "GET", "/api/v1/role-assignments?enabled=false&limit=1", nil, "")
	if len(list["items"].([]any)) != 1 {
		t.Fatal(list)
	}
	assignmentCall(t, a, 412, "DELETE", path, nil, "\"1\"")
	assignmentCall(t, a, 204, "DELETE", path, nil, "\"2\"")
	assignmentCall(t, a, 404, "GET", path, nil, "")
	tests := []struct {
		name string
		edit func(map[string]any)
	}{
		{"bad role", func(v map[string]any) { v["roleId"] = "not-a-uuid" }},
		{"missing role", func(v map[string]any) { v["roleId"] = "00000000-0000-0000-0000-000000000001" }},
		{"missing token", func(v map[string]any) {
			v["principal"] = map[string]any{"kind": "control_token", "id": "00000000-0000-0000-0000-000000000001"}
		}},
		{"bootstrap", func(v map[string]any) { v["principal"] = map[string]any{"kind": "control_token", "id": "bootstrap"} }},
		{"token issuer", func(v map[string]any) {
			v["principal"] = map[string]any{"kind": "control_token", "id": token, "issuer": "https://idp"}
		}},
		{"untrusted oidc", func(v map[string]any) {
			v["principal"] = map[string]any{"kind": "oidc", "id": "user", "issuer": "https://idp"}
		}},
		{"mixed mapping", func(v map[string]any) { v["mappingId"] = "00000000-0000-0000-0000-000000000001" }},
		{"missing mapping", func(v map[string]any) {
			delete(v, "principal")
			v["mappingId"] = "00000000-0000-0000-0000-000000000001"
		}},
		{"capture required", func(v map[string]any) { v["captureScopes"] = []string{} }},
		{"unknown capture", func(v map[string]any) { v["captureScopes"] = []string{"bad"} }},
		{"duplicate capture", func(v map[string]any) { v["captureScopes"] = []string{"control", "control"} }},
		{"tenant pin", func(v map[string]any) { v["scope"] = map[string]any{"kind": "tenant", "epoch": token} }},
		{"unknown scope", func(v map[string]any) { v["scope"] = map[string]any{"kind": "bad"} }},
		{"missing team", func(v map[string]any) { v["scope"] = map[string]any{"kind": "team", "id": token} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := assignmentInput(role, token, map[string]any{"kind": "tenant"})
			tt.edit(v)
			assignmentCall(t, a, 422, "POST", "/api/v1/role-assignments", v, "")
		})
	}
	// Disabled OIDC drafts can be staged without making an issuer trusted.
	draft := assignmentInput(role, token, map[string]any{"kind": "tenant"})
	draft["principal"] = map[string]any{"kind": "oidc", "id": "subject", "issuer": "https://unconfigured"}
	draft["enabled"] = false
	out = assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", draft, "")
	draft["enabled"] = true
	assignmentCall(t, a, 422, "PUT", "/api/v1/role-assignments/"+out["id"].(string), draft, "\"1\"")
	a.expect(200, "POST", "/api/v1/admin-tokens/payload-investigator/revoke", nil)
	input["enabled"] = true
	assignmentCall(t, a, 422, "POST", "/api/v1/role-assignments", input, "")
	input["enabled"] = false
	assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", input, "")
}

func TestAuthorizationAssignmentHistoricalScope(t *testing.T) {
	a, before, _ := controlAuditFixture(t)
	_, token := payloadReader(t, a)
	role := assignmentRole(t, a, "epoch-auditor", "audit.events.read", "audit.payloads.request.read")
	options := assignmentCall(t, a, 200, "GET", "/api/v1/access/scopes?kind=upstream", nil, "")
	items := options["items"].([]any)
	var resource, epoch string
	for _, item := range items {
		v := item.(map[string]any)
		if v["slug"] == "crm" {
			resource = v["id"].(string)
			epoch = v["epoch"].(string)
		}
	}
	if resource == "" || epoch == "" {
		t.Fatal(options)
	}
	input := assignmentInput(role, token, map[string]any{"kind": "upstream", "id": resource})
	out := assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", input, "")
	id := out["id"].(string)
	scope := out["scope"].(map[string]any)
	if scope["epoch"] != epoch || scope["ownerTeam"] != before {
		t.Fatal("scope not pinned", out)
	}
	liveRole := assignmentRole(t, a, "live-maintainer", "upstreams.update")
	live := assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", assignmentInput(liveRole, token, map[string]any{"kind": "upstream", "id": resource}), "")
	if _, has := live["scope"].(map[string]any)["epoch"]; has {
		t.Fatal("live assignment unexpectedly pinned")
	}
	// Adding audit later must not turn an unpinned live assignment into history access.
	assignmentCall(t, a, 409, "PUT", "/api/v1/roles/live-maintainer", map[string]any{"name": "Live", "permissions": []string{"upstreams.update", "audit.events.read"}}, "\"1\"")
	a.expect(200, "PATCH", "/api/v1/upstreams/crm", map[string]any{"ownerTeam": "after"})
	out = assignmentCall(t, a, 200, "PUT", "/api/v1/role-assignments/"+id, input, "\"1\"")
	if out["scope"].(map[string]any)["epoch"] != epoch {
		t.Fatal("update followed ownership transfer", out)
	}
	l := assignmentCall(t, a, 200, "GET", "/api/v1/role-assignments/"+live["id"].(string), nil, "")
	if l["enabled"] != false || l["version"] != float64(2) {
		t.Fatal("live grant survived transfer", l)
	}
	epochs := assignmentCall(t, a, 200, "GET", "/api/v1/access/resource-epochs?kind=upstream&resourceId="+resource, nil, "")
	if len(epochs["items"].([]any)) != 2 {
		t.Fatal(epochs)
	}
	p, err := store.LoadAuthorization(context.Background(), a.pool)
	if err != nil {
		t.Fatal(err)
	}
	who := authz.Principal{Kind: "control_token", ID: token}
	old := authz.Resource{Tenant: p.Tenant, Kind: "upstream", ID: resource, OwnerTeam: before, Epoch: epoch}
	q := authz.Request{Principal: who, Permission: "audit.payloads.request.read", Resource: old, Audit: true, CaptureScope: "control"}
	if !p.Decide(q).Allowed {
		t.Fatal("lost historical access")
	}
	for _, x := range epochs["items"].([]any) {
		e := x.(map[string]any)
		if e["id"] != epoch {
			q.Resource.Epoch = e["id"].(string)
			q.Resource.OwnerTeam = fmt.Sprint(e["ownerTeam"])
		}
	}
	if p.Decide(q).Allowed {
		t.Fatal("old grant sees new ownership")
	}
	// A closed resource epoch remains explicitly assignable even after deletion.
	a.expect(204, "DELETE", "/api/v1/upstreams/crm", nil)
	input["scope"] = map[string]any{"kind": "upstream", "id": resource, "epoch": epoch}
	assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", input, "")
	input["scope"] = map[string]any{"kind": "upstream", "id": resource}
	assignmentCall(t, a, 422, "POST", "/api/v1/role-assignments", input, "")
}

func TestAuthorizationOperatorAssignmentsAreRevocable(t *testing.T) {
	a := setup(t)
	_, token := payloadReader(t, a)
	_, _, role := a.do("GET", "/api/v1/roles/platform-operator", nil, nil)
	input := assignmentInput(role["id"].(string), token, map[string]any{"kind": "tenant"})
	out := assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", input, "")
	path := "/api/v1/role-assignments/" + out["id"].(string)
	input["enabled"] = false
	assignmentCall(t, a, 200, "PUT", path, input, "\"1\"")
	assignmentCall(t, a, 200, "PUT", path, assignmentInput(role["id"].(string), token, map[string]any{"kind": "tenant"}), "\"2\"")
	input["enabled"] = true
	second := assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", input, "")
	statuses := make(chan int, 2)
	for _, id := range []string{out["id"].(string), second["id"].(string)} {
		go func(id string) {
			version := "\"1\""
			if id == out["id"].(string) {
				version = "\"3\""
			}
			code, _, _ := a.do("DELETE", "/api/v1/role-assignments/"+id, nil, map[string]string{"If-Match": version})
			statuses <- code
		}(id)
	}
	one, two := <-statuses, <-statuses
	if one != 204 || two != 204 {
		t.Fatal("operator assignments must both be revocable", one, two)
	}
}

func TestAuthorizationAssignmentProjectPinAndSelectors(t *testing.T) {
	a, before, _ := controlAuditFixture(t)
	read, token := payloadReader(t, a)
	role := assignmentRole(t, a, "project-auditor", "audit.events.read")
	options := assignmentCall(t, a, 200, "GET", "/api/v1/access/scopes?kind=project", nil, "")
	project := ""
	for _, item := range options["items"].([]any) {
		v := item.(map[string]any)
		if v["slug"] == "product" {
			project = v["id"].(string)
		}
	}
	if project == "" {
		t.Fatal(options)
	}
	input := assignmentInput(role, token, map[string]any{"kind": "project", "id": project})
	out := assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", input, "")
	if out["scope"].(map[string]any)["ownerTeam"] != before {
		t.Fatal(out)
	}
	a.expect(200, "POST", "/api/v1/projects", map[string]any{"slug": "product", "name": "Product", "ownerTeam": "after", "version": 1})
	out = assignmentCall(t, a, 200, "PUT", "/api/v1/role-assignments/"+out["id"].(string), input, "\"1\"")
	if out["scope"].(map[string]any)["ownerTeam"] != before {
		t.Fatal("project grant followed new owner", out)
	}
	for _, kind := range []string{"team", "project", "upstream", "virtual_server", "control_token"} {
		assignmentCall(t, a, 200, "GET", "/api/v1/access/scopes?kind="+kind+"&limit=1", nil, "")
		code, _, _ := a.do("GET", "/api/v1/access/scopes?kind="+kind, nil, map[string]string{"Authorization": "Bearer " + read})
		if code != 403 {
			t.Fatal("scope selector leaked", kind, code)
		}
	}
	assignmentCall(t, a, 422, "GET", "/api/v1/access/scopes?kind=unknown", nil, "")
	assignmentCall(t, a, 422, "GET", "/api/v1/access/resource-epochs?kind=upstream&resourceId=bad", nil, "")
	// Filter before pagination and preserve a stable continuation cursor.
	second := assignmentInput(role, token, map[string]any{"kind": "tenant"})
	second["enabled"] = false
	assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", second, "")
	assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", second, "")
	page := assignmentCall(t, a, 200, "GET", "/api/v1/role-assignments?enabled=false&limit=1", nil, "")
	cursor, ok := page["nextCursor"].(string)
	if !ok {
		t.Fatal(page)
	}
	next := assignmentCall(t, a, 200, "GET", "/api/v1/role-assignments?enabled=false&limit=1&cursor="+cursor, nil, "")
	if next["nextCursor"] != nil || page["items"].([]any)[0].(map[string]any)["id"] == next["items"].([]any)[0].(map[string]any)["id"] {
		t.Fatal("unstable pagination", page, next)
	}
}

func TestAuthorizationAssignmentTrustedOIDCAndMappingReferences(t *testing.T) {
	a := setup(t)
	ctx := store.WithAccess(context.Background(), store.Access{Role: "admin", Actor: "test", Write: true})
	role := assignmentRole(t, a, "oidc-auditor", "audit.events.read")
	in := store.AssignmentInput{RoleID: role, Principal: authz.Principal{Kind: "oidc", Issuer: "https://trusted", ID: "subject"}, Scope: authz.Scope{Kind: "tenant"}, Enabled: true}
	var out authz.Assignment
	save := func(issuer string) error {
		return a.control.Store.Do(ctx, func(tx *store.Tx) error {
			var err error
			out, err = tx.SaveAuthorizationAssignment(ctx, "", in, nil, issuer)
			return err
		})
	}
	if err := save("https://trusted"); err != nil {
		t.Fatal(err)
	}
	if out.Principal != in.Principal {
		t.Fatal(out)
	}
	if err := save("https://other"); err == nil {
		t.Fatal("untrusted direct principal accepted")
	}
	var mapping string
	if err := a.pool.QueryRow(ctx, `INSERT INTO authorization_mappings(issuer,principal_kind,conditions) VALUES('https://trusted','oidc','[{"path":"scope","operator":"scope_contains","value":"audit:read"}]') RETURNING id::text`).Scan(&mapping); err != nil {
		t.Fatal(err)
	}
	in.Principal = authz.Principal{}
	in.MappingID = mapping
	if err := save("https://trusted"); err != nil {
		t.Fatal(err)
	}
	p, err := store.LoadAuthorization(ctx, a.pool)
	if err != nil {
		t.Fatal(err)
	}
	who := authz.Principal{Kind: "oidc", Issuer: "https://trusted", ID: "mapped-subject"}
	q := authz.Request{Principal: who, Permission: "audit.events.read", Resource: authz.Resource{Tenant: p.Tenant}}
	if p.Decide(q).Allowed {
		t.Fatal("mapping granted before resolution")
	}
	if !p.ResolveMappings(who, map[string]any{"scope": "audit:read"}).Decide(q).Allowed {
		t.Fatal("mapping assignment not linked")
	}
	if p.ResolveMappings(who, map[string]any{"scope": "audit:read:all"}).Decide(q).Allowed {
		t.Fatal("partial scope matched")
	}
}

func TestAuthorizationAssignmentChangesApplyToAuditPayloads(t *testing.T) {
	a, _, _ := controlAuditFixture(t)
	token, id := payloadReader(t, a)
	settings := settingsFor(t, a)
	settings.Scopes["control"] = audit.Capture{Request: true, Response: true}
	a.expect(200, "PUT", "/api/v1/audit-settings", settings)
	_, h, _ := a.do("PATCH", "/api/v1/upstreams/crm", map[string]any{"description": "private"}, nil)
	request := awaitControlEvent(t, a, h.Get("X-Request-Id"), "admission")
	response := awaitControlEvent(t, a, h.Get("X-Request-Id"), "outcome")
	role := assignmentRole(t, a, "api-investigator", "audit.events.read", "audit.payloads.request.read")
	in := assignmentInput(role, id, map[string]any{"kind": "tenant"})
	code, headers, out := a.do("POST", "/api/v1/role-assignments", in, nil)
	if code != 201 {
		t.Fatal(code, out)
	}
	for _, stage := range []string{"admission", "mutation", "outcome"} {
		event := awaitControlEvent(t, a, headers.Get("X-Request-Id"), stage)
		var payloads int
		if err := a.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_payloads WHERE event_id=$1`, event).Scan(&payloads); err != nil || payloads != 0 {
			t.Fatal("assignment contents captured", payloads, err)
		}
	}

	controlPayloadRead(t, a, token, request, 200)
	controlPayloadRead(t, a, token, response, 404)
	// Rollout is explicitly set by this test; creating/updating assignments never enables it.
	in["enabled"] = false
	assignmentCall(t, a, 200, "PUT", "/api/v1/role-assignments/"+out["id"].(string), in, "\"1\"")
	controlPayloadRead(t, a, token, request, 404)
}
