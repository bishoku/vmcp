package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/testdb"
	"github.com/pressly/goose/v3"
	"testing"
)

func TestOmniPublicationDefaultsAndPreservesOmittedUpdates(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	pool, err := Connect(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	box, _ := secrets.NewBox(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	s := Store{Pool: pool, Box: box}
	in := VirtualServerInput{Name: "Chat", Description: "Customer support", Enabled: true, AuthRequired: true}
	save := func() {
		t.Helper()
		if err := s.Do(ctx, func(tx *Tx) error { _, _, e := tx.PutVirtualServer(ctx, "chat", in, nil, false); return e }); err != nil {
			t.Fatal(err)
		}
	}
	save()
	v, err := s.Reader().GetVirtualServer(ctx, "chat")
	if err != nil || v.OmniEnabled {
		t.Fatalf("default %v %v", v, err)
	}
	yes := true
	in.OmniEnabled = &yes
	save()
	v, _ = s.Reader().GetVirtualServer(ctx, "chat")
	if !v.OmniEnabled {
		t.Fatal("publication not persisted")
	}
	in.OmniEnabled = nil
	in.Name = "Updated"
	save()
	v, _ = s.Reader().GetVirtualServer(ctx, "chat")
	if !v.OmniEnabled {
		t.Fatal("old client cleared publication")
	}
	no := false
	if err := s.Do(ctx, func(tx *Tx) error {
		_, e := tx.PatchVirtualServer(ctx, "chat", VirtualServerPatch{OmniEnabled: &no}, nil)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	v, _ = s.Reader().GetVirtualServer(ctx, "chat")
	if v.OmniEnabled {
		t.Fatal("unpublication failed")
	}
}

func TestOmniMigrationFromV110PreservesExistingServer(t *testing.T) {
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
	if err := goose.DownToContext(ctx, db, ".", 33); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO virtual_servers(tenant_id,slug,name) SELECT id,'existing','Existing v1.1.0' FROM tenants WHERE slug='default'`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	var enabled bool
	var name string
	if err := db.QueryRowContext(ctx, `SELECT name,omni_enabled FROM virtual_servers WHERE slug='existing'`).Scan(&name, &enabled); err != nil || enabled || name != "Existing v1.1.0" {
		t.Fatalf("upgrade changed existing server: %s %v %v", name, enabled, err)
	}
}
