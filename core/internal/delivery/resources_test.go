package delivery

import (
	"context"
	"encoding/base64"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
	"testing"
)

func TestResourceHintsAreFencedSessionScopedAndRevocable(t *testing.T) {
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
	box, _ := secrets.NewBox(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	s := &Store{Pool: pool, Box: box}
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(id,virtual_server,key_slug,protocol_version,initialized) VALUES('a','v','','2025-06-18',true),('b','v','','2025-06-18',true);INSERT INTO upstreams(tenant_id,slug,name,url,transport) SELECT id,'resource-hints','Hints','https://example.com/mcp','streamable_http' FROM tenants WHERE slug='default'`); err != nil {
		t.Fatal(err)
	}
	owner := UUID()
	if _, err := pool.Exec(ctx, `INSERT INTO mcp_resource_subscriptions(session_id,uri,upstream_id,original_uri,revision,owner,lease_until) SELECT 'a','vmcp://hints/public',id,'doc://public',1,$1,now()+interval '15 seconds' FROM upstreams WHERE slug='resource-hints'`, owner); err != nil {
		t.Fatal(err)
	}
	if err := s.ResourceUpdated(ctx, "a", "vmcp://hints/public", UUID()); err == nil {
		t.Fatal("superseded physical watcher published")
	}
	if err := s.ResourceUpdated(ctx, "b", "vmcp://hints/public", owner); err == nil {
		t.Fatal("cross-session resource hint")
	}
	if err := s.ResourceUpdated(ctx, "a", "vmcp://hints/public", owner); err != nil {
		t.Fatal(err)
	}
	stream, err := s.GetStream(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	reader := UUID()
	if err := s.Claim(ctx, "a", stream, reader); err != nil {
		t.Fatal(err)
	}
	events, err := s.Events(ctx, "a", stream, reader, 0)
	if err != nil || len(events) != 1 {
		t.Fatalf("hint queued before GET missing: %d %v", len(events), err)
	}
	if _, _, err := s.Resume(ctx, "b", events[0].ID); err == nil {
		t.Fatal("cross-session resource replay")
	}
	if _, err := pool.Exec(ctx, `UPDATE sessions SET terminated_at=now() WHERE id='a'`); err != nil {
		t.Fatal(err)
	}
	if err := s.ResourceUpdated(ctx, "a", "vmcp://hints/public", owner); err == nil {
		t.Fatal("terminated session received hint")
	}
}
