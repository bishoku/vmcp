package store

import (
	"context"
	"database/sql"
	"github.com/bishoku/vmcp/core/internal/testdb"
	"github.com/pressly/goose/v3"
	"testing"
)

func TestConversationMigrationPreservesKeysAndAudit(t *testing.T) {
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
	if err := goose.DownToContext(ctx, db, ".", 34); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO agent_keys(tenant_id,slug,key_hash,key_prefix)SELECT id,'existing',decode('01','hex'),'prefix' FROM tenants WHERE slug='default';INSERT INTO audit_events(virtual_server,method,status)VALUES('existing','tools/call','ok')`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	var mapping, id string
	if err := db.QueryRowContext(ctx, `SELECT audit_correlation::text FROM agent_keys WHERE slug='existing'`).Scan(&mapping); err != nil || mapping != "{}" {
		t.Fatalf("upgrade key: %s %v", mapping, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT conversation_id FROM audit_events WHERE virtual_server='existing'`).Scan(&id); err != nil || id != "" {
		t.Fatalf("upgrade event: %s %v", id, err)
	}
	if err := goose.DownToContext(ctx, db, ".", 34); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM audit_events WHERE virtual_server='existing'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("rollback deleted audit")
	}
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
}
