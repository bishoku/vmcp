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
	enc, err := s.Box.SealHeaders(map[string]string{"Authorization": "Bearer tok"})
	if err != nil {
		t.Fatal(err)
	}
	stmts := []struct {
		q    string
		args []any
	}{
		{`INSERT INTO upstreams (tenant_id, slug, name, url, transport, encrypted_headers)
		  SELECT id, 'github', 'GitHub', 'http://gh.example/mcp', 'streamable_http', $1 FROM tenants WHERE slug='default'`, []any{enc}},
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
