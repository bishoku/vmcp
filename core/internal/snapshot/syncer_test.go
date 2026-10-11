package snapshot_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/snapshot"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
	"github.com/jackc/pgx/v5"
)

// Requires a real Postgres: VMCP_TEST_DATABASE_URL=postgres://.../vmcp_test
func setup(t *testing.T) (context.Context, *snapshot.Syncer) {
	t.Helper()
	url := testdb.New(t)
	ctx, cancel := context.WithCancel(context.Background())

	if err := store.Migrate(ctx, url); err != nil {
		cancel()
		t.Fatal(err)
	}
	pool, err := store.Connect(ctx, url)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	// Cleanups run LIFO: cancel must run before pool.Close, otherwise Close
	// waits forever for the listener's connection to be released.
	t.Cleanup(pool.Close)
	t.Cleanup(cancel)
	for _, q := range []string{`DELETE FROM virtual_servers`, `DELETE FROM upstreams`} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}

	k := make([]byte, 32)
	_, _ = rand.Read(k)
	box, err := secrets.NewBox(base64.StdEncoding.EncodeToString(k))
	if err != nil {
		t.Fatal(err)
	}
	return ctx, &snapshot.Syncer{
		Pool: pool, Box: box,
		Log:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		PollInterval: time.Hour, // force the NOTIFY path to do the work
	}
}

func seed(t *testing.T, ctx context.Context, s *snapshot.Syncer) {
	t.Helper()
	enc, err := s.Box.SealHeadersBound(map[string]string{"Authorization": "Bearer tok"}, "00000000-0000-4000-8000-000000000002")
	if err != nil {
		t.Fatal(err)
	}
	stmts := []struct {
		q    string
		args []any
	}{
		{`INSERT INTO upstreams (id, tenant_id, slug, name, url, transport, encrypted_headers)
		  SELECT '00000000-0000-4000-8000-000000000002'::uuid, id, 'github', 'GitHub', 'http://gh.example/mcp', 'streamable_http', $1 FROM tenants WHERE slug='default'`, []any{enc}},
		{`INSERT INTO upstreams (tenant_id, slug, name, url, transport, enabled)
		  SELECT id, 'off', 'Off', 'http://off.example/mcp', 'sse', false FROM tenants WHERE slug='default'`, nil},
		{`INSERT INTO upstream_tools (upstream_id, name, description, input_schema)
		  SELECT id, 'create_issue', 'Create an issue', '{"type":"object"}' FROM upstreams WHERE slug='github'`, nil},
		{`INSERT INTO upstream_tools (upstream_id, name) SELECT id, 'x' FROM upstreams WHERE slug='off'`, nil},
		{`INSERT INTO virtual_servers (tenant_id, slug, name) SELECT id, 'support', 'Support' FROM tenants WHERE slug='default'`, nil},
		{`INSERT INTO virtual_server_tools (virtual_server_id, upstream_id, tool_name, alias)
		  SELECT v.id, u.id, 'create_issue', 'file_issue' FROM virtual_servers v, upstreams u WHERE v.slug='support' AND u.slug='github'`, nil},
		{`INSERT INTO virtual_server_tools (virtual_server_id, upstream_id, tool_name)
		  SELECT v.id, u.id, 'x' FROM virtual_servers v, upstreams u WHERE v.slug='support' AND u.slug='off'`, nil},
	}
	for _, st := range stmts {
		if _, err := s.Pool.Exec(ctx, st.q, st.args...); err != nil {
			t.Fatalf("%s: %v", st.q, err)
		}
	}
}

func TestLoadSnapshot(t *testing.T) {
	ctx, s := setup(t)
	seed(t, ctx, s)

	snap, err := store.LoadSnapshot(ctx, s.Pool, s.Box)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Upstreams) != 1 {
		t.Fatalf("disabled upstream must be excluded: %+v", snap.Upstreams)
	}
	for _, u := range snap.Upstreams {
		if u.Headers["Authorization"] != "Bearer tok" || u.URL != "http://gh.example/mcp" {
			t.Fatalf("headers not decrypted: %+v", u)
		}
	}
	vs, ok := snap.VirtualServers["support"]
	if !ok || len(vs.Tools) != 1 {
		t.Fatalf("tools of a disabled upstream must be skipped: %+v", vs)
	}
	if vs.Tools[0].Tool.Name != "file_issue" || vs.Tools[0].UpstreamName != "create_issue" {
		t.Fatalf("alias not applied: %+v", vs.Tools[0])
	}
	if string(vs.Tools[0].Tool.InputSchema) != `{"type": "object"}` {
		t.Fatalf("schema not carried: %s", vs.Tools[0].Tool.InputSchema)
	}
	if snap.Revision < 2 {
		t.Fatalf("triggers should have bumped the revision, got %d", snap.Revision)
	}
}

func TestSyncerPicksUpChangesViaNotify(t *testing.T) {
	ctx, s := setup(t)
	seed(t, ctx, s)
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	before := s.Snapshot().Revision

	if _, err := s.Pool.Exec(ctx, `INSERT INTO virtual_servers (tenant_id, slug, name)
		SELECT id, 'newbie', 'New' FROM tenants WHERE slug='default'`); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if snap := s.Snapshot(); snap.Revision > before {
			if _, ok := snap.VirtualServers["newbie"]; !ok {
				t.Fatal("new revision loaded without the new virtual server")
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("snapshot was not refreshed after a config change (poll interval is 1h, so NOTIFY must have failed)")
}

func TestFailedDecryptionExpiresAuthorization(t *testing.T) {
	ctx, s := setup(t)
	seed(t, ctx, s)
	s.MaxAuthorizationAge = 200 * time.Millisecond
	s.PollInterval = 20 * time.Millisecond
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	before := s.Snapshot().Revision
	if _, err := s.Pool.Exec(ctx, `UPDATE upstreams SET encrypted_headers = $1 WHERE slug='github'`, []byte("invalid ciphertext")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for s.Ready() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if s.Ready() || s.Status().ObservedRevision <= before {
		t.Fatalf("failed decryption kept authorization fresh: %+v", s.Status())
	}
	if snap, err := s.AuthorizedSnapshot(); err == nil || snap != nil {
		t.Fatal("stale snapshot authorized")
	}
	enc, err := s.Box.SealHeadersBound(map[string]string{"Authorization": "Bearer rotated"}, "00000000-0000-4000-8000-000000000002")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE upstreams SET encrypted_headers = $1 WHERE slug='github'`, enc); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for !s.Ready() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !s.Ready() {
		t.Fatal("authorization did not recover")
	}
}

func TestDedicatedListenerReconnectsWithoutPoolCapacity(t *testing.T) {
	t.Setenv("VMCP_DB_MAX_CONNS", "2")
	ctx, s := setup(t)
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	// Observe and terminate the listener using a separate connection: the
	// request pool will be deliberately exhausted below. Background refreshes
	// legitimately acquire pool connections, so an instantaneous idle-count
	// assertion cannot establish whether the listener uses the pool.
	observerCtx, stopObserver := context.WithTimeout(ctx, 15*time.Second)
	defer stopObserver()
	observer, err := pgx.Connect(observerCtx, s.Pool.Config().ConnConfig.ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close(observerCtx)
	deadline := time.Now().Add(3 * time.Second)
	var listeners int
	for time.Now().Before(deadline) {
		if err := observer.QueryRow(observerCtx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND query='LISTEN vmcp_config'`).Scan(&listeners); err != nil {
			t.Fatal(err)
		}
		if listeners > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if listeners != 1 {
		t.Fatalf("dedicated listener count %d", listeners)
	}
	for i := int32(0); i < s.Pool.Config().MaxConns; i++ {
		conn, err := s.Pool.Acquire(observerCtx)
		if err != nil {
			t.Fatalf("request pool capacity unavailable: %v", err)
		}
		defer conn.Release()
	}
	if s.Pool.Stat().AcquiredConns() != s.Pool.Config().MaxConns {
		t.Fatal("test did not exhaust request pool")
	}
	if _, err := observer.Exec(observerCtx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=current_database() AND query='LISTEN vmcp_config' AND pid<>pg_backend_pid()`); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := observer.QueryRow(observerCtx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND query='LISTEN vmcp_config'`).Scan(&listeners); err != nil {
			t.Fatal(err)
		}
		if s.ListenerReconnects() > 0 && listeners == 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("listener did not reconnect")
}
