package store

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/bishoku/vmcp/core/internal/authz"
	"github.com/bishoku/vmcp/core/internal/testdb"
	"github.com/bishoku/vmcp/core/migrations"
	"github.com/pressly/goose/v3"
)

func TestAuthorizationMigrationAndPolicyLoading(t *testing.T) {
	ctx := context.Background()
	url := testdb.New(t)
	// Start from the actual preceding version, rather than only a fresh database.
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	migrationMu.Lock()
	goose.SetBaseFS(migrations.FS)
	err = goose.SetDialect("postgres")
	if err == nil {
		err = goose.UpToContext(ctx, db, ".", 35)
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
	if _, err = pool.Exec(ctx, `INSERT INTO audit_events(method,status,virtual_server) VALUES('legacy-check','ok','old-server')`); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, url); err != nil {
		t.Fatal("repeat migration", err)
	}
	p, err := LoadAuthorization(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Assignments) != 0 {
		t.Fatal("migration created implicit grants", p.Assignments)
	}
	var obsolete int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='authorization_state' AND column_name IN ('mode','audit_payload_mode')`).Scan(&obsolete); err != nil || obsolete != 0 {
		t.Fatal("authorization bypass selectors remain", obsolete, err)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM authorization_permissions`).Scan(&count); err != nil || count != len(authz.Catalog()) {
		t.Fatal("catalog mismatch", count, err)
	}
	for _, spec := range authz.Catalog() {
		var scopes []string
		var capture bool
		if err = pool.QueryRow(ctx, `SELECT scopes,capture_restricted FROM authorization_permissions WHERE id=$1`, spec.ID).Scan(&scopes, &capture); err != nil {
			t.Fatal(spec.ID, err)
		}
		if !slices.Equal(scopes, spec.Scopes) || capture != spec.Capture {
			t.Fatalf("catalog seed drift: %s", spec.ID)
		}
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM audit_access_contexts`).Scan(&count); err != nil || count != 0 {
		t.Fatal("legacy events backfilled", count, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE method='legacy-check'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("legacy event lost", err)
	}
	var roleID string
	if err = pool.QueryRow(ctx, `SELECT id::text FROM authorization_roles WHERE slug='observer'`).Scan(&roleID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO authorization_assignments(role_id,principal_kind,issuer,principal_id,scope_kind) VALUES($1,'oidc','https://idp','alice','tenant')`, roleID); err != nil {
		t.Fatal(err)
	}
	next, err := LoadAuthorization(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if next.Revision <= p.Revision {
		t.Fatal("revision not invalidated")
	}
	who := authz.Principal{Kind: "oidc", Issuer: "https://idp", ID: "alice"}
	q := authz.Request{Principal: who, Permission: "upstreams.read", Resource: authz.Resource{Tenant: next.Tenant}}
	if !next.Decide(q).Allowed {
		t.Fatal("loaded assignment not effective")
	}
	if _, err = pool.Exec(ctx, `UPDATE authorization_assignments SET enabled=false`); err != nil {
		t.Fatal(err)
	}
	revoked, err := LoadAuthorization(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Revision <= next.Revision || revoked.Decide(q).Allowed {
		t.Fatal("revocation stale")
	}
	// Audit provenance is additive, tenant-bound, and cannot be retargeted.
	var eventID, epochID string
	if err = pool.QueryRow(ctx, `SELECT event_id::text FROM audit_events WHERE method='legacy-check'`).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO audit_resource_epochs(resource_kind,resource_id,start_revision) VALUES('virtual_server','11111111-1111-1111-1111-111111111111',1) RETURNING epoch_id::text`).Scan(&epochID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO audit_access_contexts(event_id,classification,origin,operation_config_revision,provenance_state) VALUES($1,'resource','test',1,'verified')`, eventID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO audit_access_domains(event_id,domain_kind,resource_kind,resource_id,epoch_id) VALUES($1,'consumer','virtual_server','22222222-2222-2222-2222-222222222222',$2)`, eventID, epochID); err == nil {
		t.Fatal("mismatched epoch accepted")
	}
	if _, err = pool.Exec(ctx, `INSERT INTO audit_access_domains(event_id,domain_kind,resource_kind,resource_id,epoch_id) VALUES($1,'consumer','virtual_server','11111111-1111-1111-1111-111111111111',$2)`, eventID, epochID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE audit_access_contexts SET classification='tenant' WHERE event_id=$1`, eventID); err == nil {
		t.Fatal("provenance rewrite accepted")
	}
	if _, err = pool.Exec(ctx, `UPDATE audit_resource_epochs SET resource_id=gen_random_uuid() WHERE epoch_id=$1`, epochID); err == nil {
		t.Fatal("epoch rewrite accepted")
	}
	if _, err = pool.Exec(ctx, `UPDATE audit_resource_epochs SET end_revision=2 WHERE epoch_id=$1`, epochID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE audit_resource_epochs SET end_revision=3 WHERE epoch_id=$1`, epochID); err == nil {
		t.Fatal("closed epoch rewrite accepted")
	}
	if _, err = pool.Exec(ctx, `INSERT INTO authorization_assignments(role_id,principal_kind,principal_id,scope_kind) VALUES($1,'oidc','alice','tenant')`, roleID); err == nil {
		t.Fatal("issuer-less OIDC principal accepted")
	}
	if _, err = pool.Exec(ctx, `INSERT INTO authorization_assignments(role_id,principal_kind,principal_id,scope_kind,capture_scopes) VALUES($1,'control_token','token','tenant',ARRAY['unknown'])`, roleID); err == nil {
		t.Fatal("unknown capture scope accepted")
	}
	// Down/up only this additive migration, leaving legacy rows intact.
	migrationMu.Lock()
	err = goose.DownToContext(ctx, db, ".", 35)
	migrationMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE method='legacy-check'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("rollback damaged legacy data", err)
	}
}

func TestAuthorizationDatabaseErrorsDeny(t *testing.T) {
	if _, err := LoadAuthorization(context.Background(), nil); err == nil {
		t.Fatal("nil database accepted")
	}
}
