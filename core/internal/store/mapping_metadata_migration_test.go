package store

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	"github.com/bishoku/vmcp/core/internal/authz"
	"github.com/bishoku/vmcp/core/internal/testdb"
	"github.com/bishoku/vmcp/core/migrations"
	"github.com/pressly/goose/v3"
)

func TestMappingMetadataMigrationPreservesExistingGrants(t *testing.T) {
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
	step(46, false)
	pool, err := Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `INSERT INTO authorization_mappings(issuer,principal_kind,conditions,enabled) VALUES('https://trusted.example','oidc','[{"path":"groups","operator":"array_contains","value":"support"}]',true);
 INSERT INTO authorization_assignments(role_id,mapping_id,scope_kind,enabled) SELECT r.id,m.id,'tenant',true FROM authorization_roles r CROSS JOIN authorization_mappings m WHERE r.slug='observer'`); err != nil {
		t.Fatal(err)
	}
	before, err := LoadAuthorization(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	step(47, false)
	after, err := LoadAuthorization(ctx, pool)
	if err != nil || !reflect.DeepEqual(before.Mappings, after.Mappings) || !reflect.DeepEqual(before.Assignments, after.Assignments) {
		t.Fatal("migration changed existing identity grants", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE authorization_mappings SET name='Support team',description='Support staff'`); err != nil {
		t.Fatal(err)
	}
	named, err := LoadAuthorization(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	who := authz.Principal{Kind: "oidc", Issuer: "https://trusted.example", ID: "sample"}
	for _, claims := range []map[string]any{{"groups": []any{"support"}}, {"groups": []any{"other"}}} {
		if !reflect.DeepEqual(before.ResolveMappings(who, claims).Assignments, named.ResolveMappings(who, claims).Assignments) {
			t.Fatal("renaming changed resolved assignments")
		}
	}
	step(46, true)
	restored, err := LoadAuthorization(ctx, pool)
	if err != nil || !reflect.DeepEqual(before.Mappings, restored.Mappings) || !reflect.DeepEqual(before.Assignments, restored.Assignments) {
		t.Fatal("rollback changed grants", err)
	}
}
