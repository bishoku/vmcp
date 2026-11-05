package store

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"testing"

	"github.com/bishoku/vmcp/core/internal/authz"
	"github.com/bishoku/vmcp/core/internal/testdb"
	"github.com/bishoku/vmcp/core/migrations"
	"github.com/pressly/goose/v3"
)

func TestPlatformOperatorMigrationPreservesIdentityAssignmentsAndCustomCopies(t *testing.T) {
	ctx := context.Background()
	url := testdb.New(t)
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	step := func(version int64, down bool) {
		t.Helper()
		migrationMu.Lock()
		defer migrationMu.Unlock()
		goose.SetBaseFS(migrations.FS)
		if err := goose.SetDialect("postgres"); err != nil {
			t.Fatal(err)
		}
		var err error
		if down {
			err = goose.DownToContext(ctx, db, ".", version)
		} else {
			err = goose.UpToContext(ctx, db, ".", version)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	step(41, false)
	pool, err := Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, `INSERT INTO authorization_roles(slug,name) VALUES('independent-copy','Independent copy');
 INSERT INTO authorization_role_permissions(role_id,permission_id) SELECT c.id,p.permission_id FROM authorization_roles c CROSS JOIN authorization_roles r JOIN authorization_role_permissions p ON p.role_id=r.id WHERE c.slug='independent-copy' AND r.slug='platform-admin';
 INSERT INTO authorization_assignments(role_id,principal_kind,issuer,principal_id,scope_kind,capture_scopes) SELECT id,'oidc','https://trusted.example','operator','tenant',ARRAY['control'] FROM authorization_roles WHERE slug IN ('platform-admin','independent-copy')`); err != nil {
		t.Fatal(err)
	}
	before, err := LoadAuthorization(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	find := func(p authz.Policy, slug string) authz.Role {
		t.Helper()
		for _, r := range p.Roles {
			if r.Slug == slug {
				return r
			}
		}
		t.Fatal("role missing", slug)
		return authz.Role{}
	}
	old := find(before, "platform-admin")
	step(42, false)
	after, err := LoadAuthorization(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	renamed := find(after, "platform-operator")
	if renamed.ID != old.ID || renamed.Version != old.Version+1 || renamed.Name != "Platform Operator" || !renamed.Builtin || after.Revision <= before.Revision {
		t.Fatal("operator identity/version not preserved", renamed)
	}
	expected := []string{}
	for _, p := range old.Permissions {
		if !strings.HasPrefix(p, "access.") {
			expected = append(expected, p)
		}
	}
	if !reflect.DeepEqual(expected, renamed.Permissions) {
		t.Fatal("unrelated operator permissions changed")
	}
	if !reflect.DeepEqual(find(before, "independent-copy"), find(after, "independent-copy")) || !reflect.DeepEqual(before.Assignments, after.Assignments) {
		t.Fatal("custom copy or assignments rewritten")
	}
	// Real admin token identity is held outside role assignments, and no migration
	// synthesizes an administrator credential or login mapping.
	var mappings, tokens int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM authorization_mappings),(SELECT count(*) FROM admin_tokens)`).Scan(&mappings, &tokens); err != nil || mappings != 0 || tokens != 0 {
		t.Fatal("migration created login identities", mappings, tokens, err)
	}
	step(41, true)
	restored, err := LoadAuthorization(ctx, pool)
	if err != nil || !reflect.DeepEqual(before.Roles, restored.Roles) || !reflect.DeepEqual(before.Assignments, restored.Assignments) {
		t.Fatal("rollback changed policy", err)
	}
	step(42, false)
	replay, err := LoadAuthorization(ctx, pool)
	if err != nil || !reflect.DeepEqual(after.Roles, replay.Roles) || !reflect.DeepEqual(after.Assignments, replay.Assignments) {
		t.Fatal("replay changed policy", err)
	}
}
