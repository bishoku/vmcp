package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/bishoku/vmcp/core/internal/testdb"
	"github.com/bishoku/vmcp/core/migrations"
	"github.com/pressly/goose/v3"
)

// v2.0.0 ends at migration 42. Preserve existing application routing and customer
// context while introducing opt-in invocation policy and durable replay storage.
func TestInvocationUpgradeFromV200(t *testing.T) {
	ctx := context.Background()
	url := testdb.New(t)
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	migrationMu.Lock()
	goose.SetBaseFS(migrations.FS)
	err = goose.SetDialect("postgres")
	if err == nil {
		err = goose.UpToContext(ctx, db, ".", 42)
	}
	migrationMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	pool, err := Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	_, err = pool.Exec(ctx, `INSERT INTO upstreams(tenant_id,slug,name,url,transport,encrypted_headers,caller_context)
 SELECT id,'existing','Existing','https://existing.example/mcp','streamable_http',decode('010203','hex'),
 '{"enabled":true,"metaKey":"app/customer","fields":[{"name":"sessionId","required":true}]}'::jsonb FROM tenants WHERE slug='default';
 INSERT INTO authorization_roles(slug,name) VALUES('custom-reader','Custom reader');
 INSERT INTO authorization_role_permissions(role_id,permission_id) SELECT id,'upstreams.read' FROM authorization_roles WHERE slug='custom-reader';
 INSERT INTO authorization_assignments(role_id,principal_kind,issuer,principal_id,scope_kind)
 SELECT id,'oidc','https://issuer.example','operator','tenant' FROM authorization_roles WHERE slug='custom-reader'`)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		err := pool.QueryRow(ctx, `SELECT jsonb_build_object('upstreams',(SELECT jsonb_agg(to_jsonb(u)) FROM upstreams u),
   'assignments',(SELECT jsonb_agg(to_jsonb(a)) FROM authorization_assignments a),
   'customPermissions',(SELECT jsonb_agg(p.permission_id ORDER BY p.permission_id) FROM authorization_role_permissions p JOIN authorization_roles r ON r.id=p.role_id WHERE r.slug='custom-reader'))::text`).Scan(&value)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	if err := CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if snapshot() != before {
		t.Fatal("upgrade changed existing routing, context or custom grants")
	}
	settings, err := loadInvocationSecurity(ctx, pool, nil)
	if err != nil || len(settings.Services) != 0 || len(settings.Upstreams) != 0 {
		t.Fatal("upgrade enabled invocation requirements", err)
	}
	var grants int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM authorization_role_permissions p JOIN authorization_roles r ON r.id=p.role_id WHERE r.slug='platform-operator' AND p.permission_id LIKE 'invocation_security.%'`).Scan(&grants); err != nil || grants != 2 {
		t.Fatal("operator permissions missing", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO invocation_dispatches(operation_id,binding_hash) VALUES('operation','binding')`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO invocation_dispatches(operation_id,binding_hash) VALUES('operation','other')`); err == nil {
		t.Fatal("migration rerun lost replay reservation")
	}
}
