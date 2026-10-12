package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/bishoku/vmcp/core/internal/testdb"
	"github.com/pressly/goose/v3"
)

func TestDeliveryMigrationUpgradeAndRollback(t *testing.T) {
	ctx := context.Background()
	url := testdb.New(t)
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := goose.DownToContext(ctx, db, ".", 18); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sessions(id,virtual_server,key_slug,protocol_version) VALUES('delivery-upgrade','v','','2025-06-18')`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	var table string
	if err := db.QueryRowContext(ctx, `SELECT 'mcp_events'::regclass::text`).Scan(&table); err != nil {
		t.Fatal(err)
	}
	if err := goose.DownToContext(ctx, db, ".", 18); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sessions WHERE id='delivery-upgrade'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("rollback lost session: %d %v", count, err)
	}
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
}

func TestSessionLifecycleMigrationUpgradeAndRollback(t *testing.T) {
	url := testdb.New(t)
	ctx := context.Background()
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Return to schema 15, preserving a pre-upgrade client session.
	if err := goose.DownToContext(ctx, db, ".", 15); err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO sessions (id, virtual_server, key_slug, protocol_version) VALUES ('old-session','agent','key','2025-03-26')`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	var initialized bool
	var caps, version string
	if err := db.QueryRowContext(ctx, `SELECT initialized, client_capabilities::text, protocol_version FROM sessions WHERE id = 'old-session'`).Scan(&initialized, &caps, &version); err != nil {
		t.Fatal(err)
	}
	if initialized || caps != "{}" || version != "2025-03-26" {
		t.Fatalf("unexpected migrated session: %v %s %s", initialized, caps, version)
	}
	if err := goose.DownToContext(ctx, db, ".", 15); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sessions WHERE id = 'old-session'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("rollback lost session: %d %v", count, err)
	}
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
}

func TestSessionOwnershipMigrationUpgradeAndRollback(t *testing.T) {
	url := testdb.New(t)
	ctx := context.Background()
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := goose.DownToContext(ctx, db, ".", 16); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sessions(id,virtual_server,key_slug,protocol_version,initialized) VALUES('legacy','agent','oauth:shared','2025-06-18',true)`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	var mode, subject string
	var terminated bool
	if err := db.QueryRowContext(ctx, `SELECT owner_mode,owner_subject,terminated_at IS NOT NULL FROM sessions WHERE id='legacy'`).Scan(&mode, &subject, &terminated); err != nil {
		t.Fatal(err)
	}
	if mode != "legacy" || subject != "" || terminated {
		t.Fatalf("unsafe inferred ownership: %s %s %v", mode, subject, terminated)
	}
	if _, err := db.ExecContext(ctx, `UPDATE sessions SET terminated_at=now() WHERE id='legacy'`); err != nil {
		t.Fatal(err)
	}
	if err := goose.DownToContext(ctx, db, ".", 16); err != nil {
		t.Fatal(err)
	}
	var initialized bool
	if err := db.QueryRowContext(ctx, `SELECT initialized FROM sessions WHERE id='legacy'`).Scan(&initialized); err != nil || !initialized {
		t.Fatalf("rollback lost lifecycle: %v %v", initialized, err)
	}
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
}

func TestDurableAuditMigrationUpgradeAndRollback(t *testing.T) {
	url := testdb.New(t)
	ctx := context.Background()
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := goose.DownToContext(ctx, db, ".", 17); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO audit_events(virtual_server,method,status) VALUES('agent','tools/call','ok')`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	var version int
	var eventID, stage string
	if err := db.QueryRowContext(ctx, `SELECT event_id::text,schema_version,stage FROM audit_events`).Scan(&eventID, &version, &stage); err != nil || eventID == "" || version != 1 || stage != "event" {
		t.Fatalf("legacy audit upgrade: %s %d %s %v", eventID, version, stage, err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO audit_events(virtual_server,method,status,stage,operation_id,request_id) VALUES('agent','tools/call','started','admission',gen_random_uuid(),gen_random_uuid())`); err != nil {
		t.Fatal(err)
	}
	if err := goose.DownToContext(ctx, db, ".", 17); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM audit_events WHERE status='ok'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("rollback lost old audit: %d %v", count, err)
	}
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogMigrationUpgradeAndRollback(t *testing.T) {
	ctx := context.Background()
	url := testdb.New(t)
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := goose.DownToContext(ctx, db, ".", 19); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO upstreams(tenant_id,slug,name,url,transport) SELECT id,'catalog-upgrade','Catalog','https://example.com/mcp','streamable_http' FROM tenants WHERE slug='default'`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	var caps string
	if err := db.QueryRowContext(ctx, `SELECT mcp_capabilities::text FROM upstreams WHERE slug='catalog-upgrade'`).Scan(&caps); err != nil || caps != "{}" {
		t.Fatalf("upgrade defaults %s %v", caps, err)
	}
	if err := goose.DownToContext(ctx, db, ".", 19); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM upstreams WHERE slug='catalog-upgrade'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("catalog rollback lost upstream")
	}
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
}

func TestClientFeatureMigrationUpgradeAndRollback(t *testing.T) {
	ctx := context.Background()
	url := testdb.New(t)
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := goose.DownToContext(ctx, db, ".", 20); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sessions(id,virtual_server,key_slug,protocol_version) VALUES('features-upgrade','v','','2025-06-18')`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	var level string
	if err := db.QueryRowContext(ctx, `SELECT logging_level FROM sessions WHERE id='features-upgrade'`).Scan(&level); err != nil || level != "warning" {
		t.Fatalf("unsafe logging default %s %v", level, err)
	}
	if err := goose.DownToContext(ctx, db, ".", 20); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sessions WHERE id='features-upgrade'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("feature rollback lost session")
	}
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPOnlyMigrationPreservesUnsupportedConfigurations(t *testing.T) {
	ctx := context.Background()
	url := testdb.New(t)
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := goose.DownToContext(ctx, db, ".", 25); err != nil {
		t.Fatal(err)
	}
	// Simulate a previously deployed schema allowing a process transport.
	if _, err := db.ExecContext(ctx, `ALTER TABLE upstreams DROP CONSTRAINT upstreams_transport_check;
 ALTER TABLE upstreams ADD CONSTRAINT upstreams_transport_check CHECK(transport IN ('streamable_http','sse','process'));
 INSERT INTO upstreams(tenant_id,slug,name,url,transport) SELECT id,'old-process','Old process','https://example.com/mcp','process' FROM tenants WHERE slug='default'`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, url); err == nil {
		t.Fatal("upgrade accepted unsupported configuration")
	}
	var transport string
	if err := db.QueryRowContext(ctx, `SELECT transport FROM upstreams WHERE slug='old-process'`).Scan(&transport); err != nil || transport != "process" {
		t.Fatalf("failed upgrade changed configuration: %q %v", transport, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE upstreams SET transport='streamable_http' WHERE slug='old-process'`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE upstreams SET transport='process' WHERE slug='old-process'`); err == nil {
		t.Fatal("database accepted unsupported transport")
	}
	if err := db.QueryRowContext(ctx, `SELECT transport FROM upstreams WHERE slug='old-process'`).Scan(&transport); err != nil || transport != "streamable_http" {
		t.Fatalf("HTTP configuration lost: %q %v", transport, err)
	}
}
