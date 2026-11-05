package authz

import "testing"

var person = Principal{Kind: "oidc", Issuer: "https://idp", ID: "alice"}

func fixture(permissions ...string) Policy {
	return Policy{Tenant: "t", Revision: 1, Roles: []Role{{ID: "r", Permissions: permissions}}, Assignments: []Assignment{{ID: "a", RoleID: "r", Principal: person, Scope: Scope{Kind: "team", ID: "crm"}, Enabled: true, CaptureScopes: []string{"virtual", "upstream"}}}}
}
func TestAssignmentScopeAndDefaultDeny(t *testing.T) {
	p := fixture("upstreams.update")
	q := Request{Principal: person, Permission: "upstreams.update", Resource: Resource{Tenant: "t", Kind: "upstream", ID: "u", OwnerTeam: "crm"}}
	if !p.Decide(q).Allowed {
		t.Fatal("own scope denied")
	}
	for _, mutate := range []func(*Request){func(q *Request) { q.Resource.OwnerTeam = "other" }, func(q *Request) { q.Resource.Tenant = "other" }, func(q *Request) { q.Permission = "unknown" }, func(q *Request) { q.Principal.Issuer = "evil" }, func(q *Request) { q.Principal.Kind = "unknown" }} {
		copy := q
		mutate(&copy)
		if p.Decide(copy).Allowed {
			t.Fatalf("unexpected allow: %+v", copy)
		}
	}
	// A separate broad reader assignment cannot lend its scope to the editor.
	p.Roles = append(p.Roles, Role{ID: "reader", Permissions: []string{"upstreams.read"}})
	p.Assignments = append(p.Assignments, Assignment{ID: "b", RoleID: "reader", Principal: person, Enabled: true, Scope: Scope{Kind: "tenant"}})
	q.Resource.OwnerTeam = "other"
	if p.Decide(q).Allowed {
		t.Fatal("scope laundering")
	}
	p.Revision = 0
	if p.Decide(q).Allowed {
		t.Fatal("unavailable policy allowed")
	}
}
func TestAuditPayloadDirectionsEpochsAndLegacy(t *testing.T) {
	p := fixture("audit.events.read", "audit.payloads.request.read")
	e := AuditPayload{Classification: "resource", Tenant: "t", Direction: "request", CaptureScope: "virtual", Domains: []Resource{{Tenant: "t", Kind: "virtual_server", ID: "v", OwnerTeam: "crm", Epoch: "old"}}}
	if !p.CanReadPayload(person, e) {
		t.Fatal("request denied")
	}
	e.Direction = "response"
	if p.CanReadPayload(person, e) {
		t.Fatal("response leaked")
	}
	e.Direction = "request"
	e.CaptureScope = "control"
	if p.CanReadPayload(person, e) {
		t.Fatal("control payload leaked")
	}
	e.CaptureScope = "virtual"
	p.Assignments[0].Scope = Scope{Kind: "virtual_server", ID: "v", Epoch: "new"}
	if p.CanReadPayload(person, e) {
		t.Fatal("new owner inherited old epoch")
	}
	p.Assignments[0].Scope.Epoch = "old"
	if !p.CanReadPayload(person, e) {
		t.Fatal("explicit historical grant denied")
	}
	e.Classification = "legacy"
	if p.CanReadPayload(person, e) {
		t.Fatal("scoped legacy payload leaked")
	}
	p.Assignments[0].Scope = Scope{Kind: "tenant"}
	if p.CanReadPayload(person, e) {
		t.Fatal("legacy gate missing")
	}
	p.Roles[0].Permissions = append(p.Roles[0].Permissions, "audit.legacy.events.read", "audit.legacy.payloads.read")
	if !p.CanReadPayload(person, e) {
		t.Fatal("explicit legacy request denied")
	}
	e.Direction = ""
	if p.CanReadPayload(person, e) {
		t.Fatal("unclassified payload leaked")
	}
	p.Roles[0].Permissions = append(p.Roles[0].Permissions, "audit.legacy.payloads.unclassified.read")
	if !p.CanReadPayload(person, e) {
		t.Fatal("explicit unclassified denied")
	}
	e.DeniedDiagnostics = true
	if p.CanReadPayload(person, e) {
		t.Fatal("diagnostics bypass")
	}
}
func TestAuditAllDomainsAndCaptureRestrictions(t *testing.T) {
	p := fixture("audit.events.read", "audit.payloads.response.read")
	e := AuditPayload{Classification: "resource", Tenant: "t", Direction: "response", CaptureScope: "upstream", Domains: []Resource{{Tenant: "t", OwnerTeam: "crm"}, {Tenant: "t", OwnerTeam: "other"}}}
	if p.CanReadPayload(person, e) {
		t.Fatal("partial domains allowed")
	}
	e.Domains = e.Domains[:1]
	if !p.CanReadPayload(person, e) {
		t.Fatal("consumer denied")
	}
	p.Assignments[0].CaptureScopes = nil
	if p.CanReadPayload(person, e) {
		t.Fatal("empty capture allowlist grants access")
	}
}
func TestTypedMapping(t *testing.T) {
	p := fixture("upstreams.read")
	p.Mappings = []Mapping{{ID: "m", Issuer: person.Issuer, PrincipalKind: "oidc", Enabled: true, Conditions: []Condition{{Path: "realm.roles", Operator: "array_contains", Value: "crm"}, {Path: "scope", Operator: "scope_contains", Value: "config.read"}}}}
	p.Assignments[0].Principal = Principal{}
	p.Assignments[0].MappingID = "m"
	claims := map[string]any{"realm": map[string]any{"roles": []any{"crm"}}, "scope": "config.read config.other"}
	q := Request{Principal: person, Permission: "upstreams.read", Resource: Resource{Tenant: "t", OwnerTeam: "crm"}}
	if p.Decide(q).Allowed {
		t.Fatal("unresolved mapping allowed")
	}
	if !p.ResolveMappings(person, claims).Decide(q).Allowed {
		t.Fatal("verified mapping denied")
	}
	claims["scope"] = "config.read.all"
	if p.ResolveMappings(person, claims).Decide(q).Allowed {
		t.Fatal("substring scope match")
	}
	if p.Assignments[0].MappingID != "m" {
		t.Fatal("input mutated")
	}
}
func TestCatalog(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range Catalog() {
		if seen[p.ID] || p.ID == "" || len(p.Scopes) == 0 {
			t.Fatalf("invalid catalog entry %+v", p)
		}
		seen[p.ID] = true
	}
	p, _ := Lookup("audit.payloads.request.read")
	p.Scopes[0] = "evil"
	q, _ := Lookup("audit.payloads.request.read")
	if q.Scopes[0] == "evil" {
		t.Fatal("mutable catalog")
	}
}

func TestProjectHistoryAndTenantIsolation(t *testing.T) {
	p := fixture("audit.events.read", "audit.payloads.response.read")
	p.Assignments[0].Scope = Scope{Kind: "project", ID: "project", OwnerTeam: "old-team"}
	e := AuditPayload{Classification: "resource", Tenant: "t", Direction: "response", CaptureScope: "virtual", Domains: []Resource{{Tenant: "t", Kind: "virtual_server", ID: "v", Project: "project", OwnerTeam: "old-team", Epoch: "e1"}}}
	if !p.CanReadPayload(person, e) {
		t.Fatal("project historical grant denied")
	}
	p.Assignments[0].Scope.OwnerTeam = "new-team"
	if p.CanReadPayload(person, e) {
		t.Fatal("project transfer inherited old messages")
	}
	p.Assignments[0].Scope = Scope{Kind: "unmanaged"}
	e.Domains = nil
	e.Classification = "tenant"
	if p.CanReadPayload(person, e) {
		t.Fatal("unmanaged grant expanded to tenant")
	}
	e.Domains = []Resource{{Tenant: "other", Kind: "virtual_server"}}
	if p.CanReadPayload(person, e) {
		t.Fatal("cross tenant domain")
	}
}

func TestMalformedPolicyFailsClosed(t *testing.T) {
	p := fixture("upstreams.read")
	p.Roles[0].Permissions = append(p.Roles[0].Permissions, "unknown.permission")
	q := Request{Principal: person, Permission: "upstreams.read", Resource: Resource{Tenant: "t", OwnerTeam: "crm"}}
	if p.Validate() == nil || p.Decide(q).Allowed {
		t.Fatal("malformed policy grants access")
	}
	p = fixture("upstreams.read")
	p.Assignments[0].Enabled = false
	if p.Decide(q).Allowed {
		t.Fatal("disabled assignment")
	}
}

func TestMissingProvenanceNeverDefaultsToTenant(t *testing.T) {
	p := fixture("audit.events.read", "audit.payloads.request.read")
	p.Assignments[0].Scope = Scope{Kind: "tenant"}
	e := AuditPayload{Tenant: "t", Direction: "request", CaptureScope: "virtual"}
	if p.CanReadPayload(person, e) {
		t.Fatal("missing provenance bypassed legacy gate")
	}
	e.Classification = "resource"
	if p.CanReadPayload(person, e) {
		t.Fatal("resource event without domains")
	}
	e.Classification = "tenant"
	if !p.CanReadPayload(person, e) {
		t.Fatal("explicit tenant event denied")
	}
}

func TestHistoricalEpochGrantCannotManageTransferredResource(t *testing.T) {
	p := fixture("virtual_servers.update")
	p.Assignments[0].Scope = Scope{Kind: "virtual_server", ID: "server", Epoch: "old"}
	q := Request{Principal: person, Permission: "virtual_servers.update", Resource: Resource{Tenant: "t", Kind: "virtual_server", ID: "server", Epoch: "new"}}
	if p.Decide(q).Allowed {
		t.Fatal("historical grant became live management after transfer")
	}
}
