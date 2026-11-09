//go:build acceptance

package e2e

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
)

func recoveryPGEnv(t *testing.T, connection string) []string {
	t.Helper()
	u, err := url.Parse(connection)
	if err != nil {
		t.Fatal("invalid recovery database URL")
	}
	env := []string{}
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "PG") {
			env = append(env, value)
		}
	}
	password, _ := u.User.Password()
	return append(env, "PGHOST="+u.Hostname(), "PGPORT="+u.Port(), "PGUSER="+u.User.Username(), "PGPASSWORD="+password, "PGDATABASE="+strings.TrimPrefix(u.Path, "/"), "PGSSLMODE="+u.Query().Get("sslmode"))
}

// TestLogicalRecoveryAcceptance restores a real logical backup into a separate
// throwaway database. It is not PostgreSQL promotion, physical PITR or an RTO SLA.
func TestLogicalRecoveryAcceptance(t *testing.T) {
	if os.Getenv("VMCP_RUN_RECOVERY_ACCEPTANCE") != "1" {
		t.Fatal("explicit recovery acceptance opt-in required")
	}
	output, keyDir := os.Getenv("VMCP_RECOVERY_OUTPUT"), os.Getenv("VMCP_RECOVERY_KEY_DIR")
	if output == "" || keyDir == "" {
		t.Fatal("separate evidence and retained recovery-key directories required")
	}
	if filepath.Clean(output) == filepath.Clean(keyDir) {
		t.Fatal("recovery keys must be separate from evidence")
	}
	for _, dir := range []string{output, keyDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	source, target := testdb.New(t), testdb.New(t)
	if err := store.Migrate(ctx, source); err != nil {
		t.Fatal(err)
	}
	pool, err := store.Connect(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	keys := map[string]string{}
	for _, id := range []string{"old", "new"} {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			t.Fatal(err)
		}
		keys[id] = base64.StdEncoding.EncodeToString(raw)
	}
	keyJSON, _ := json.Marshal(keys)
	keyFile, err := os.OpenFile(filepath.Join(keyDir, "recovery-keyring.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = keyFile.Write(keyJSON); err != nil {
		keyFile.Close()
		t.Fatal(err)
	}
	if err = keyFile.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VMCP_TENANT_ID", "readiness-fixture")
	t.Setenv("VMCP_ALLOW_LEGACY_CIPHERTEXT", "false")
	t.Setenv("VMCP_CIPHERTEXT_WRITE_VERSION", "v1")
	t.Setenv("VMCP_DECRYPT_KEYS", string(keyJSON))
	for i, id := range []string{"old", "new"} {
		t.Setenv("VMCP_KEY_ID", id)
		box, err := secrets.FromEnv(keys[id])
		if err != nil {
			t.Fatal(err)
		}
		rowID := []string{"00000000-0000-4000-8000-000000000055", "00000000-0000-4000-8000-000000000056"}[i]
		ciphertext, err := box.SealHeadersBound(map[string]string{"Authorization": "synthetic-recovery-fixture"}, rowID)
		if err != nil {
			t.Fatal(err)
		}
		_, err = pool.Exec(ctx, `INSERT INTO upstreams(id,tenant_id,slug,name,url,transport,encrypted_headers) SELECT $1,id,$2,'Recovery fixture','https://example.invalid','streamable_http',$3 FROM tenants WHERE slug='default'`, rowID, id, ciphertext)
		if err != nil {
			t.Fatal(err)
		}
	}
	backup := filepath.Join(output, "fixture.dump")
	dump, err := os.OpenFile(backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	commandCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	start := time.Now()
	binDir := os.Getenv("VMCP_RECOVERY_PG_BIN")
	cmd := exec.CommandContext(commandCtx, filepath.Join(binDir, "pg_dump"), "--format=custom", "--no-owner", "--no-acl")
	cmd.Env = recoveryPGEnv(t, source)
	cmd.Stdout = dump
	err = cmd.Run()
	closeErr := dump.Close()
	if err != nil {
		t.Fatalf("pg_dump failed: %v", err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	backupSeconds := time.Since(start).Seconds()
	start = time.Now()
	cmd = exec.CommandContext(commandCtx, filepath.Join(binDir, "pg_restore"), "--dbname=", "--no-owner", "--no-acl", "--exit-on-error", backup)
	cmd.Env = recoveryPGEnv(t, target)
	if err = cmd.Run(); err != nil {
		t.Fatalf("pg_restore failed: %v", err)
	}
	restored, err := store.Connect(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	t.Setenv("VMCP_KEY_ID", "new")
	box, err := secrets.FromEnv(keys["new"])
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("VMCP_TENANT_ID", "different-tenant")
	wrongTenant, err := secrets.FromEnv(keys["new"])
	if err != nil {
		t.Fatal(err)
	}
	rows, err := restored.Query(ctx, `SELECT id::text,encrypted_headers FROM upstreams ORDER BY slug`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id string
		var ciphertext []byte
		if err = rows.Scan(&id, &ciphertext); err != nil {
			t.Fatal(err)
		}
		headers, err := box.OpenHeadersBound(ciphertext, id)
		if err != nil || headers["Authorization"] != "synthetic-recovery-fixture" {
			t.Fatal("restored configuration decryption failed")
		}
		if _, err = box.OpenHeadersBound(ciphertext, "other-row"); err == nil {
			t.Fatal("AAD transplant accepted")
		}
		if _, err = wrongTenant.OpenHeadersBound(ciphertext, id); err == nil {
			t.Fatal("cross-tenant transplant accepted")
		}
		count++
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("restored rows: %d", count)
	}
	restoreSeconds := time.Since(start).Seconds()
	data, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	result := map[string]any{"kind": "logical backup, not PITR", "fixture_rows": count, "old_and_new_keys_decrypt": true, "wrong_row_and_tenant_rejected": true, "backup_seconds": backupSeconds, "restore_and_decrypt_seconds": restoreSeconds, "backup_sha256": hex.EncodeToString(hash[:]), "backup_bytes": len(data), "production_rpo_rto": "not measured", "application_http_functionality": "not tested"}
	encoded, _ := json.MarshalIndent(result, "", "  ")
	if err = os.WriteFile(filepath.Join(output, "recovery.json"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
}
