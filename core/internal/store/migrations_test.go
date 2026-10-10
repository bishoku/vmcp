package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/bishoku/vmcp/core/internal/testdb"
	"github.com/pressly/goose/v3"
)

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
	if err := goose.DownContext(ctx, db, "."); err != nil {
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
	if err := goose.DownContext(ctx, db, "."); err != nil {
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
