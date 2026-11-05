package controlplane_test

import (
	"context"
	"testing"
)

func configurationGrant(t *testing.T, a *api, token string, permissions []string, kind string, id, epoch any) {
	t.Helper()
	var role string
	if err := a.pool.QueryRow(context.Background(), `INSERT INTO authorization_roles(slug,name) VALUES('config-test','Config test') RETURNING id::text`).Scan(&role); err != nil {
		t.Fatal(err)
	}
	for _, p := range permissions {
		if _, err := a.pool.Exec(context.Background(), `INSERT INTO authorization_role_permissions(role_id,permission_id) VALUES($1,$2)`, role, p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.pool.Exec(context.Background(), `INSERT INTO authorization_assignments(role_id,principal_kind,principal_id,scope_kind,scope_id,epoch_id) VALUES($1,'control_token',$2,$3,$4,$5)`, role, token, kind, id, epoch); err != nil {
		t.Fatal(err)
	}
}
func TestConfigurationScopedReadAndFieldPermissions(t *testing.T) {
	a, team, _ := controlAuditFixture(t)
	token, id := payloadReader(t, a)
	a.expect(201, "POST", "/api/v1/upstreams", map[string]any{"slug": "aaa-hidden", "name": "Hidden", "url": a.fixture.URL, "ownerTeam": "after"})
	configurationGrant(t, a, id, []string{"upstreams.read", "upstreams.update"}, "team", team, nil)

	headers := map[string]string{"Authorization": "Bearer " + token}
	code, _, page := a.do("GET", "/api/v1/upstreams?limit=1", nil, headers)
	if code != 200 {
		t.Fatal(code, page)
	}
	items := page["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["slug"] != "crm" || page["nextCursor"] != nil {
		t.Fatal("scope must precede pagination", page)
	}
	code, _, _ = a.do("GET", "/api/v1/upstreams/aaa-hidden", nil, headers)
	if code != 403 && code != 404 {
		t.Fatal("cross-team metadata exposed", code)
	}
	code, _, out := a.do("PATCH", "/api/v1/upstreams/crm", map[string]any{"name": "Renamed"}, headers)
	if code != 200 {
		t.Fatal("rename", code, out)
	}
	for _, patch := range []map[string]any{{"url": a.fixture.URL}, {"enabled": false}, {"headers": map[string]string{"Authorization": "private"}}, {"tls": nil}, {"callerContext": nil}, {"ownerTeam": "after"}} {
		code, _, _ = a.do("PATCH", "/api/v1/upstreams/crm", patch, headers)
		if code != 403 {
			t.Fatal("protected field accepted", patch, code)
		}
	}
	code, _, _ = a.do("DELETE", "/api/v1/upstreams/crm", nil, headers)
	if code != 403 {
		t.Fatal("update became delete", code)
	}
	code, _, _ = a.do("GET", "/api/v1/virtual-servers", nil, headers)
	if code != 403 {
		t.Fatal("upstream grant became VS read", code)
	}
	a.expect(200, "GET", "/api/v1/upstreams/crm", nil) // explicit compatibility admin still works
}
func TestConfigurationCredentialAllTargetsAndEmptyGrants(t *testing.T) {
	a, team, _ := controlAuditFixture(t)
	token, id := payloadReader(t, a)
	a.expect(200, "POST", "/api/v1/projects", map[string]any{"slug": "other", "name": "Other", "ownerTeam": "after"})
	a.expect(201, "POST", "/api/v1/virtual-servers", map[string]any{"slug": "outside", "name": "Outside", "project": "other"})
	a.expect(201, "POST", "/api/v1/agent-keys", map[string]any{"slug": "shared", "virtualServers": []string{"chat", "outside"}})
	a.expect(201, "POST", "/api/v1/agent-keys", map[string]any{"slug": "inside", "virtualServers": []string{"chat"}})
	configurationGrant(t, a, id, []string{"agent_keys.read", "agent_keys.rotate", "agent_keys.update", "agent_keys.access.update", "virtual_servers.read"}, "team", team, nil)

	headers := map[string]string{"Authorization": "Bearer " + token}
	code, _, out := a.do("GET", "/api/v1/agent-keys?limit=1", nil, headers)
	if code != 200 {
		t.Fatal(code, out)
	}
	items := out["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["slug"] != "inside" {
		t.Fatal("cross-target key listed", out)
	}
	for _, path := range []string{"/api/v1/agent-keys/shared", "/api/v1/agent-keys/shared/rotate"} {
		method := "GET"
		if path[len(path)-6:] == "rotate" {
			method = "POST"
		}
		code, _, _ = a.do(method, path, nil, headers)
		if code != 403 {
			t.Fatal("cross-target credential", path, code)
		}
	}
	code, _, out = a.do("POST", "/api/v1/agent-keys/inside/rotate", nil, headers)
	if code != 200 {
		t.Fatal("scoped rotation", code, out)
	}
	code, _, _ = a.do("PATCH", "/api/v1/agent-keys/inside", map[string]any{"virtualServers": []string{"chat", "outside"}}, headers)
	if code != 403 {
		t.Fatal("credential scope widened", code)
	}
	code, _, _ = a.do("PATCH", "/api/v1/agent-keys/inside", map[string]any{"virtualServers": []string{}}, headers)
	if code != 403 {
		t.Fatal("empty credential became scoped", code)
	}
}
func TestConfigurationCreationUsesDestinationAndProtectsDefaults(t *testing.T) {
	a, team, _ := controlAuditFixture(t)
	token, id := payloadReader(t, a)
	configurationGrant(t, a, id, []string{"upstreams.create", "upstreams.connection.update"}, "team", team, nil)

	headers := map[string]string{"Authorization": "Bearer " + token}
	code, _, out := a.do("POST", "/api/v1/upstreams", map[string]any{"slug": "new-owned", "name": "New owned", "url": a.fixture.URL, "ownerTeam": "before"}, headers)
	if code != 201 {
		t.Fatal(code, out)
	}
	code, _, _ = a.do("POST", "/api/v1/upstreams", map[string]any{"slug": "new-unmanaged", "name": "No owner", "url": a.fixture.URL}, headers)
	if code != 403 {
		t.Fatal("create escaped destination", code)
	}
	code, _, _ = a.do("PUT", "/api/v1/upstreams/new-owned", map[string]any{"name": "Replacement", "url": a.fixture.URL, "ownerTeam": "before"}, headers)
	if code != 403 {
		t.Fatal("PUT defaults bypassed fields", code)
	}
}

func TestConfigurationVirtualFieldsAndTransferDestination(t *testing.T) {
	a, team, after := controlAuditFixture(t)
	token, id := payloadReader(t, a)
	a.expect(200, "POST", "/api/v1/projects", map[string]any{"slug": "other", "name": "Other", "ownerTeam": "after"})
	configurationGrant(t, a, id, []string{"virtual_servers.read", "virtual_servers.update", "ownership.transfer"}, "team", team, nil)

	headers := map[string]string{"Authorization": "Bearer " + token}
	code, _, out := a.do("PATCH", "/api/v1/virtual-servers/chat", map[string]any{"description": "Allowed edit"}, headers)
	if code != 200 {
		t.Fatal(code, out)
	}
	for _, patch := range []map[string]any{{"enabled": false}, {"authRequired": false}, {"pii": nil}, {"clientFeatures": map[string]any{}}, {"project": "other"}} {
		code, _, _ = a.do("PATCH", "/api/v1/virtual-servers/chat", patch, headers)
		if code != 403 {
			t.Fatal("protected VS field", patch, code)
		}
	}
	// Target-domain transfer authority is independent of source management rights.
	if _, err := a.pool.Exec(context.Background(), `INSERT INTO authorization_assignments(role_id,principal_kind,principal_id,scope_kind,scope_id) SELECT id,'control_token',$1,'team',$2 FROM authorization_roles WHERE slug='config-test'`, id, after); err != nil {
		t.Fatal(err)
	}
	code, _, out = a.do("PATCH", "/api/v1/virtual-servers/chat", map[string]any{"project": "other"}, headers)
	if code != 200 {
		t.Fatal("authorized transfer", code, out)
	}
	// Old team-only access does not see the transferred current record.
	if _, err := a.pool.Exec(context.Background(), `DELETE FROM authorization_assignments WHERE scope_id=$1`, after); err != nil {
		t.Fatal(err)
	}
	code, _, _ = a.do("GET", "/api/v1/virtual-servers/chat", nil, headers)
	if code != 403 && code != 404 {
		t.Fatal("transferred record followed old owner", code)
	}
}
func TestConfigurationNoGlobalAdminMintingAndNoAuditBypass(t *testing.T) {
	a := setup(t)
	token, id := payloadReader(t, a)
	configurationGrant(t, a, id, []string{"control_tokens.create", "control_tokens.access.update", "control_tokens.rotate", "upstreams.read"}, "tenant", nil, nil)

	headers := map[string]string{"Authorization": "Bearer " + token}
	code, _, _ := a.do("POST", "/api/v1/admin-tokens", map[string]any{"slug": "escalation", "scope": "admin"}, headers)
	if code != 403 {
		t.Fatal("minted platform admin", code)
	}
	code, _, out := a.do("POST", "/api/v1/admin-tokens", map[string]any{"slug": "new-reader", "scope": "read"}, headers)
	if code != 201 {
		t.Fatal("read identity create", code, out)
	}
	a.expect(201, "POST", "/api/v1/admin-tokens", map[string]any{"slug": "existing-admin", "scope": "admin"})
	code, _, _ = a.do("POST", "/api/v1/admin-tokens/existing-admin/rotate", nil, headers)
	if code != 403 {
		t.Fatal("rotated higher privileged identity", code)
	}
	req := payloadEvent(t, a, "request")
	code, _, _ = a.do("GET", "/api/v1/audit-events/"+req+"/payload", nil, headers)
	if code != 404 {
		t.Fatal("configuration permission became payload access", code)
	}

	code, _, _ = a.do("GET", "/api/v1/roles", nil, headers)
	if code != 403 {
		t.Fatal("tenant config role delegated management", code)
	}
}
func TestConfigurationAliasFieldsAndPolicyRevocationBeforeMutation(t *testing.T) {
	a, team, _ := controlAuditFixture(t)
	token, id := payloadReader(t, a)
	configurationGrant(t, a, id, []string{"upstreams.read", "upstreams.update"}, "team", team, nil)

	headers := map[string]string{"Authorization": "Bearer " + token}
	code, _, _ := a.do("PATCH", "/api/v1/upstreams/crm", map[string]any{"URL": a.fixture.URL}, headers)
	if code != 403 {
		t.Fatal("case alias bypass", code)
	}
	code, _, _ = a.do("PATCH", "/api/v1/upstreams/crm", map[string]any{"URL": a.fixture.URL, "url": a.fixture.URL}, headers)
	if code != 422 {
		t.Fatal("ambiguous case accepted", code)
	}
	if _, err := a.pool.Exec(context.Background(), `CREATE FUNCTION revoke_config_before_mutation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.stage='admission' AND NEW.method='PATCH' THEN UPDATE authorization_assignments SET enabled=false WHERE role_id=(SELECT id FROM authorization_roles WHERE slug='config-test'); END IF; RETURN NEW; END $$; CREATE TRIGGER revoke_config_before_mutation BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION revoke_config_before_mutation()`); err != nil {
		t.Fatal(err)
	}
	code, _, out := a.do("PATCH", "/api/v1/upstreams/crm", map[string]any{"name": "Must not commit"}, headers)
	if code != 412 {
		t.Fatal("revocation not rechecked", code, out)
	}
	var name string
	if err := a.pool.QueryRow(context.Background(), `SELECT name FROM upstreams WHERE slug='crm'`).Scan(&name); err != nil || name == "Must not commit" {
		t.Fatal("revoked mutation committed", name, err)
	}
}
func TestConfigurationGitOpsRequiresIndependentAuthority(t *testing.T) {
	a := setup(t)
	token, id := payloadReader(t, a)
	configurationGrant(t, a, id, []string{"config.export", "config.apply", "config.validate"}, "tenant", nil, nil)

	headers := map[string]string{"Authorization": "Bearer " + token}
	code, _, _ := a.do("GET", "/api/v1/export", nil, headers)
	if code != 403 {
		t.Fatal("export bypassed independent read permissions", code)
	}
	doc := map[string]any{"apiVersion": "vmcp/v1", "upstreams": []any{map[string]any{"slug": "not-allowed", "name": "No write", "url": a.fixture.URL}}, "virtualServers": []any{}}
	code, _, _ = a.do("POST", "/api/v1/apply?dryRun=true&refreshTools=false", doc, headers)
	if code != 403 {
		t.Fatal("apply bypassed per-resource actions", code)
	}
	code, _, _ = a.do("POST", "/api/v1/apply?prune=true&refreshTools=false", map[string]any{"apiVersion": "vmcp/v1", "upstreams": []any{}, "virtualServers": []any{}}, headers)
	if code != 403 {
		t.Fatal("apply became prune", code)
	}
	// Explicit platform administrators retain tenant GitOps authority.
	a.expect(200, "GET", "/api/v1/export", nil)
}

func TestConfigurationProjectReplacementCannotClearOwnershipWithoutTransfer(t *testing.T) {
	a, _, _ := controlAuditFixture(t)
	token, id := payloadReader(t, a)
	var project string
	if err := a.pool.QueryRow(context.Background(), `SELECT id::text FROM projects WHERE slug='product'`).Scan(&project); err != nil {
		t.Fatal(err)
	}
	configurationGrant(t, a, id, []string{"projects.manage"}, "project", project, nil)

	headers := map[string]string{"Authorization": "Bearer " + token}
	for _, body := range []map[string]any{{"slug": "product", "name": "Project", "version": 1}, {"slug": "product", "name": "Project", "ownerTeam": nil, "version": 1}} {
		code, _, out := a.do("POST", "/api/v1/projects", body, headers)
		if code != 403 {
			t.Fatal("replacement cleared owner", code, out)
		}
	}
	var owned bool
	if err := a.pool.QueryRow(context.Background(), `SELECT owner_team_id IS NOT NULL FROM projects WHERE slug='product'`).Scan(&owned); err != nil || !owned {
		t.Fatal(owned, err)
	}
}
func TestConfigurationBindingReplacementCannotCreateAnOrphan(t *testing.T) {
	a, team, _ := controlAuditFixture(t)
	token, id := payloadReader(t, a)
	a.expect(201, "POST", "/api/v1/identity-bindings", map[string]any{"slug": "bound", "conditions": []map[string]any{{"claim": "groups", "op": "contains", "value": "dev"}}, "virtualServers": []string{"chat"}})
	configurationGrant(t, a, id, []string{"identity_bindings.read", "identity_bindings.update", "identity_bindings.rules.update", "identity_bindings.access.update", "identity_bindings.constraints.update"}, "team", team, nil)

	headers := map[string]string{"Authorization": "Bearer " + token}
	code, _, out := a.do("PUT", "/api/v1/identity-bindings/bound", map[string]any{"conditions": []map[string]any{{"claim": "groups", "op": "contains", "value": "dev"}}}, headers)
	if code != 403 {
		t.Fatal("omitted targets cleared without tenant permission", code, out)
	}
}
