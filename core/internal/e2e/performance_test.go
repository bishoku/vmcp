package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/snapshot"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/telemetry"
	"github.com/bishoku/vmcp/core/internal/upstream"
	"github.com/bishoku/vmcp/core/internal/virtual"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type performanceQueries struct{ queries atomic.Int64 }

func (q *performanceQueries) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	q.queries.Add(1)
	return ctx
}
func (*performanceQueries) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
func (*performanceQueries) TraceBatchStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceBatchStartData) context.Context {
	return ctx
}
func (q *performanceQueries) TraceBatchQuery(context.Context, *pgx.Conn, pgx.TraceBatchQueryData) {
	q.queries.Add(1)
}
func (*performanceQueries) TraceBatchEnd(context.Context, *pgx.Conn, pgx.TraceBatchEndData) {}

// Synthetic tools only. Each sub-benchmark owns a disposable migrated database;
// persistent sessions, authoritative session reads, agent-key checks and durable
// audit admissions/outcomes remain enabled. No retry hides a failed call.
func BenchmarkPersistentToolCalls(b *testing.B) {
	for _, size := range []int{1, 100} {
		b.Run(fmt.Sprintf("upstreams=%d/servers=%d/tools-per-server=100", size, size), func(b *testing.B) {
			e := setup(b)
			e.exec(b, `INSERT INTO upstreams(tenant_id,slug,name,url,transport) SELECT t.id,'perf-up-'||n,'Performance fixture',u.url,u.transport FROM tenants t CROSS JOIN generate_series(1,$1) n CROSS JOIN upstreams u WHERE t.slug='default' AND u.slug='alpha'`, size)
			e.exec(b, `INSERT INTO upstream_tools(upstream_id,name,description,input_schema) SELECT id,'tool-'||n,'Synthetic metadata','{"type":"object"}'::jsonb FROM upstreams CROSS JOIN generate_series(1,100) n WHERE slug LIKE 'perf-up-%'`)
			e.exec(b, `INSERT INTO virtual_servers(tenant_id,slug,name) SELECT id,'perf-vs-'||n,'Performance fixture' FROM tenants CROSS JOIN generate_series(1,$1) n WHERE slug='default'`, size)
			e.exec(b, `INSERT INTO virtual_server_tools(virtual_server_id,upstream_id,tool_name) SELECT v.id,u.id,ut.name FROM virtual_servers v JOIN upstreams u ON replace(v.slug,'perf-vs-','')=replace(u.slug,'perf-up-','') JOIN upstream_tools ut ON ut.upstream_id=u.id WHERE v.slug LIKE 'perf-vs-%'`)
			token := "synthetic-performance-key"
			e.exec(b, `UPDATE virtual_servers SET auth_required=true WHERE slug='agent'`)
			e.exec(b, `INSERT INTO agent_keys(tenant_id,slug,key_hash,key_prefix) SELECT id,'performance-key',$1,'synthetic' FROM tenants WHERE slug='default'`, store.HashKey(token))
			e.exec(b, `INSERT INTO agent_key_servers(agent_key_id,virtual_server_id) SELECT k.id,v.id FROM agent_keys k CROSS JOIN virtual_servers v WHERE k.slug='performance-key' AND v.slug='agent'`)
			queries := &performanceQueries{}
			cfg := e.pool.Config()
			cfg.ConnConfig.Tracer = queries
			pool, err := pgxpool.NewWithConfig(e.ctx, cfg)
			if err != nil {
				b.Fatal(err)
			}
			defer pool.Close()
			ctx, cancel := context.WithCancel(e.ctx)
			defer cancel()
			syncer := &snapshot.Syncer{Pool: pool, Box: e.syncer.Box, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), PollInterval: time.Second}
			if err = syncer.Start(ctx); err != nil {
				b.Fatal(err)
			}
			manager := virtual.NewManager()
			manager.Store = virtual.PGSessionStore{Pool: pool}
			defer manager.Close()
			metrics := telemetry.New()
			mux := http.NewServeMux()
			recorder := audit.NewDurable(pool, nil)
			(&virtual.Handler{Snap: syncer, Sessions: manager, Audit: recorder, Metrics: metrics, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Register(mux)
			server := httptest.NewServer(metrics.Handler(mux))
			defer server.Close()
			clients := make([]*upstream.Client, runtime.GOMAXPROCS(0))
			for i := range clients {
				c := upstream.NewClient(server.URL+"/mcp/v1/virtual/agent", map[string]string{"Authorization": "Bearer " + token}, nil)
				clients[i] = c
				defer c.Close(ctx)
				if _, err = c.Initialize(ctx); err != nil {
					b.Fatal(err)
				}
				if _, err = c.CallTool(ctx, "alpha__add", json.RawMessage(`{"a":1,"b":2}`)); err != nil {
					b.Fatal(err)
				}
			}
			slots := make(chan struct{}, 8) // Keep the existing 16-request principal budget unchanged.
			var next atomic.Int64
			var mu sync.Mutex
			samples := make([]float64, 0, 10000)
			initialSQL := queries.queries.Load()
			initialAcquires := pool.Stat().AcquireCount()
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				client := clients[next.Add(1)-1]
				for pb.Next() {
					slots <- struct{}{}
					start := time.Now()
					_, err := client.CallTool(ctx, "alpha__add", json.RawMessage(`{"a":1,"b":2}`))
					elapsed := float64(time.Since(start)) / float64(time.Millisecond)
					<-slots
					if err != nil {
						b.Errorf("tool call failed: %v", err)
						return
					}
					mu.Lock()
					if len(samples) < cap(samples) {
						samples = append(samples, elapsed)
					}
					mu.Unlock()
				}
			})
			b.StopTimer()
			b.ReportMetric(8, "max-inflight")
			sort.Float64s(samples)
			if len(samples) > 0 {
				b.ReportMetric(samples[(len(samples)-1)*95/100], "p95-ms")
				b.ReportMetric(samples[(len(samples)-1)*99/100], "p99-ms")
			}
			b.ReportMetric(float64(queries.queries.Load()-initialSQL)/float64(b.N), "sql/call")
			b.ReportMetric(float64(pool.Stat().AcquireCount()-initialAcquires)/float64(b.N), "db-acquires/call")
			pending, err := audit.Pending(ctx, pool)
			if err != nil || pending.Incomplete != 0 {
				b.Fatalf("incomplete audit: %+v %v", pending, err)
			}
		})
	}
}
