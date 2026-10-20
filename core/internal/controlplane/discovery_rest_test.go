package controlplane

import (
	"context"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
	"testing"
)

func TestDiscoveryQueueExcludesRESTAndFreshMCP(t *testing.T) {
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
	_, err = pool.Exec(ctx, `INSERT INTO upstreams(tenant_id,slug,name,url,kind,transport) SELECT t.id,x.slug,x.slug,'https://example.com',x.kind,'streamable_http' FROM tenants t CROSS JOIN (VALUES ('mcp','mcp'),('rest','rest'),('fresh','mcp')) AS x(slug,kind) WHERE t.slug='default'; INSERT INTO upstream_status(upstream_id,last_refresh_at) SELECT id,now() FROM upstreams WHERE slug='fresh'`)
	if err != nil {
		t.Fatal(err)
	}
	s := Server{Store: &store.Store{Pool: pool}}
	if err := s.enqueueDiscovery(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	var slug string
	if err := pool.QueryRow(ctx, `SELECT count(*),min(u.slug) FROM mcp_discovery_requests q JOIN upstreams u ON u.id=q.upstream_id`).Scan(&count, &slug); err != nil || count != 1 || slug != "mcp" {
		t.Fatalf("queued %d %s: %v", count, slug, err)
	}
}
