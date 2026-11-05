package store

import (
	"context"
	"database/sql"
	"reflect"
	"slices"
	"testing"

	"github.com/bishoku/vmcp/core/internal/testdb"
	"github.com/bishoku/vmcp/core/migrations"
	"github.com/pressly/goose/v3"
)

func TestBuiltinWorkflowMigrationPreservesCopiesAndAssignments(t *testing.T) {
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
	step(40, false)
	pool, err := Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, `INSERT INTO authorization_roles(slug,name) VALUES('frozen-copy','Frozen');
 INSERT INTO authorization_role_permissions(role_id,permission_id) SELECT c.id,p.permission_id FROM authorization_roles c CROSS JOIN authorization_roles r JOIN authorization_role_permissions p ON p.role_id=r.id WHERE c.slug='frozen-copy' AND r.slug='credential-manager';
 INSERT INTO authorization_assignments(role_id,principal_kind,issuer,principal_id,scope_kind) SELECT id,'oidc','https://trusted.example','test-user','tenant' FROM authorization_roles WHERE slug='credential-manager'`); err != nil {
		t.Fatal(err)
	}
	before, err := LoadAuthorization(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	step(41, false)
	after, err := LoadAuthorization(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision <= before.Revision || !reflect.DeepEqual(before.Assignments, after.Assignments) {
		t.Fatal("revision or assignments changed incorrectly")
	}
	changed := map[string]string{"credential-manager": "virtual_servers.read", "virtual-server-editor": "projects.read", "automation-deployer": "audit.settings.read"}
	for _, old := range before.Roles {
		for _, current := range after.Roles {
			if current.ID != old.ID {
				continue
			}
			if permission, ok := changed[old.Slug]; ok {
				if current.Version != old.Version+1 || !slices.Contains(current.Permissions, permission) {
					t.Fatal("missing versioned dependency", current)
				}
				for _, denied := range []string{"audit.payloads.request.read", "audit.payloads.response.read", "ownership.transfer", "config.prune", "access.roles.manage"} {
					if slices.Contains(current.Permissions, denied) {
						t.Fatal("template gained unrelated privilege", current.Slug, denied)
					}
				}
			} else if !reflect.DeepEqual(old, current) {
				t.Fatal("custom copy or unrelated role changed", old.Slug)
			}
		}
	}
	step(40, true)
	restored, err := LoadAuthorization(ctx, pool)
	if err != nil || !reflect.DeepEqual(before.Roles, restored.Roles) || !reflect.DeepEqual(before.Assignments, restored.Assignments) {
		t.Fatal("rollback failed to restore templates", err)
	}
	step(41, false)
	replay, err := LoadAuthorization(ctx, pool)
	if err != nil || !reflect.DeepEqual(after.Roles, replay.Roles) || !reflect.DeepEqual(after.Assignments, replay.Assignments) {
		t.Fatal("replay changed policy", err)
	}
}
