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

func TestEnforcedMigrationPreservesAssignmentsAndUnclassifiedHistory(t *testing.T) {
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
		err = goose.UpToContext(ctx, db, ".", 39)
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
	if _, err = pool.Exec(ctx, `UPDATE authorization_state SET mode='legacy',audit_payload_mode='shadow';
 INSERT INTO authorization_roles(slug,name) VALUES('major-reader','Major reader');
 INSERT INTO authorization_role_permissions(role_id,permission_id) SELECT id,'upstreams.read' FROM authorization_roles WHERE slug='major-reader';
 INSERT INTO authorization_assignments(role_id,principal_kind,issuer,principal_id,scope_kind) SELECT id,'oidc','https://trusted.example','reader','tenant' FROM authorization_roles WHERE slug='major-reader';
 INSERT INTO audit_events(method,status,virtual_server) VALUES('major-history','ok','')`); err != nil {
		t.Fatal(err)
	}
	before, err := LoadAuthorization(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	migrate40 := func() error {
		migrationMu.Lock()
		defer migrationMu.Unlock()
		return goose.UpToContext(ctx, db, ".", 40)
	}
	if err = migrate40(); err != nil {
		t.Fatal(err)
	}
	after, err := LoadAuthorization(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision <= before.Revision || !reflect.DeepEqual(before.Assignments, after.Assignments) || !reflect.DeepEqual(before.Roles, after.Roles) {
		t.Fatal("policy lost during selector removal", before, after)
	}
	who := authz.Principal{Kind: "oidc", Issuer: "https://trusted.example", ID: "reader"}
	if !after.Decide(authz.Request{Principal: who, Permission: "upstreams.read", Resource: authz.Resource{Tenant: after.Tenant, Kind: "upstream"}}).Allowed {
		t.Fatal("explicit grant lost")
	}
	var contexts, events int
	if err = pool.QueryRow(ctx, `SELECT count(*),count(c.event_id) FROM audit_events e LEFT JOIN audit_access_contexts c USING(event_id) WHERE e.method='major-history'`).Scan(&events, &contexts); err != nil || events != 1 || contexts != 0 {
		t.Fatal("historical access was reclassified", events, contexts, err)
	}
	migrationMu.Lock()
	err = goose.DownToContext(ctx, db, ".", 39)
	migrationMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	var config, audit string
	if err = pool.QueryRow(ctx, `SELECT mode,audit_payload_mode FROM authorization_state`).Scan(&config, &audit); err != nil || config != "enforced" || audit != "enforced" {
		t.Fatal("schema rollback disabled enforcement", config, audit, err)
	}
	if err = migrate40(); err != nil {
		t.Fatal(err)
	}
	repeated, err := LoadAuthorization(ctx, pool)
	if err != nil || !reflect.DeepEqual(after.Assignments, repeated.Assignments) {
		t.Fatal("upgrade replay changed assignments", err)
	}
}
