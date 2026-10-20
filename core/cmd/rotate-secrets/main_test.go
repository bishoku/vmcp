package main

import (
	"context"
	"encoding/base64"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
	"testing"
)

func TestRestartableRotationPreservesBoundRecords(t *testing.T) {
	ctx := context.Background()
	url := testdb.New(t)
	if err := store.Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := store.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	oldKey := base64.StdEncoding.EncodeToString(make([]byte, 32))
	k := make([]byte, 32)
	k[0] = 1
	newKey := base64.StdEncoding.EncodeToString(k)
	legacy, _ := secrets.NewKeyring("old", map[string]string{"old": oldKey}, true)
	encrypted, _ := legacy.SealHeadersBound(map[string]string{"Authorization": "opaque"}, "unused")
	id := "00000000-0000-4000-8000-000000000004"
	if _, err := pool.Exec(ctx, `INSERT INTO upstreams(id,tenant_id,slug,name,url,transport,encrypted_headers) SELECT $1,id,'rotate','Rotate','https://example.com','streamable_http',$2 FROM tenants WHERE slug='default'`, id, encrypted); err != nil {
		t.Fatal(err)
	}
	restSecret := []byte(`{"auth":{"type":"bearer","token":"rest-private"}}`)
	restCiphertext, _ := legacy.SealBound(restSecret, id, "upstream.rest_config")
	if _, err := pool.Exec(ctx, `UPDATE upstreams SET encrypted_rest_config=$2 WHERE id=$1`, id, restCiphertext); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VMCP_DATABASE_URL", url)
	t.Setenv("VMCP_MASTER_KEY", newKey)
	t.Setenv("VMCP_KEY_ID", "new")
	t.Setenv("VMCP_DECRYPT_KEYS", `{"old":"`+oldKey+`"}`)
	t.Setenv("VMCP_CIPHERTEXT_WRITE_VERSION", "v1")
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if err := run(); err != nil {
		t.Fatal("restart failed", err)
	}
	var ciphertext []byte
	if err := pool.QueryRow(ctx, `SELECT encrypted_headers FROM upstreams WHERE id=$1`, id).Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	restored, _ := secrets.NewKeyring("new", map[string]string{"new": newKey}, false)
	if headers, err := restored.OpenHeadersBound(ciphertext, id); err != nil || headers["Authorization"] != "opaque" {
		t.Fatal("restore failed", err)
	}
	if err := pool.QueryRow(ctx, `SELECT encrypted_rest_config FROM upstreams WHERE id=$1`, id).Scan(&restCiphertext); err != nil {
		t.Fatal(err)
	}
	if value, err := restored.OpenBound(restCiphertext, id, "upstream.rest_config"); err != nil || string(value) != string(restSecret) {
		t.Fatal("REST secret rotation failed", err)
	}
	if _, err := restored.OpenBound(restCiphertext, "other", "upstream.rest_config"); err == nil {
		t.Fatal("REST secret transplanted")
	}
	if _, err := restored.OpenHeadersBound(ciphertext, "other"); err == nil {
		t.Fatal("rotated ciphertext transplanted")
	}
}
