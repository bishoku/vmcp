package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/bishoku/vmcp/core/internal/testdb"
	"github.com/pressly/goose/v3"
)

func TestClaimSessionMigrationRollbackRetainsOnlyLegacyRows(t *testing.T) {
	ctx := context.Background()
	url := testdb.New(t)
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, `INSERT INTO browser_sessions(id,subject,expires_at) VALUES('old','subject',now()+interval '1 hour'); INSERT INTO browser_sessions(id,subject,issuer,expires_at,model_version,session_ciphertext) VALUES('new','subject','https://idp',now()+interval '1 hour',1,'\x0102')`); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	migrationMu.Lock()
	err = goose.DownToContext(ctx, db, ".", 38)
	migrationMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM browser_sessions WHERE id='old'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("legacy row removed", count, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM browser_sessions WHERE id='new'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("reference record survived downgrade", count, err)
	}
	if err = Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	var version int
	var issuer string
	if err = pool.QueryRow(ctx, `SELECT model_version,issuer FROM browser_sessions WHERE id='old'`).Scan(&version, &issuer); err != nil || version != 0 || issuer != "" {
		t.Fatal("old row gained provenance", version, issuer, err)
	}
}
