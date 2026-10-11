package e2e

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/upstream"
	"github.com/bishoku/vmcp/core/internal/virtual"
)

// TestOperationalLoad is a reproducible small mixed-workload baseline, not a capacity claim.
func TestOperationalLoad(t *testing.T) {
	e := setup(t)
	manager := virtual.NewManager()
	manager.Store = virtual.PGSessionStore{Pool: e.pool}
	defer manager.Close()
	recorder := audit.NewDurable(e.pool, nil)
	mux := http.NewServeMux()
	(&virtual.Handler{Snap: e.syncer, Sessions: manager, Audit: recorder, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Register(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithCancel(e.ctx)
	defer cancel()
	var churn, queries, conflicts atomic.Int64
	// Start churn and calls together after all sessions complete their handshake.
	// This baseline measures call conflicts, not handshake retry behavior.
	begin := make(chan struct{})
	var initialized sync.WaitGroup
	initialized.Add(8)
	var background sync.WaitGroup
	background.Add(1)
	go func() {
		defer background.Done()
		<-begin
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if _, err := e.pool.Exec(ctx, `UPDATE virtual_servers SET name=name WHERE slug='agent'`); err == nil {
					churn.Add(1)
				}
				if _, err := audit.Pending(ctx, e.pool); err == nil {
					queries.Add(1)
				}
			}
		}
	}()
	var wg sync.WaitGroup
	var mu sync.Mutex
	var latencies, initializations []float64
	var failures atomic.Int64
	start := time.Now()
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := upstream.NewClient(server.URL+"/mcp/v1/virtual/agent", nil, nil)
			initStart := time.Now()
			_, initErr := client.Initialize(e.ctx)
			initialized.Done()
			defer client.Close(e.ctx)
			if err := initErr; err != nil {
				failures.Add(1)
				t.Logf("initialize failed: %v", err)
				return
			}
			mu.Lock()
			initializations = append(initializations, time.Since(initStart).Seconds())
			mu.Unlock()
			<-begin
			for i := 0; i < 20; i++ {
				callStart := time.Now()
				var err error
				// A revision change before dispatch intentionally returns 409.
				// Under race instrumentation five fixed 10ms retries can all
				// overlap the 20ms churn. Bound retries by time and back off.
				deadline := time.Now().Add(5 * time.Second)
				for attempt := 0; ; attempt++ {
					_, err = client.CallTool(e.ctx, "alpha__add", json.RawMessage(`{"a":1,"b":2}`))
					if err == nil || !strings.Contains(err.Error(), "returned 409: configuration changed; retry") || time.Now().After(deadline) {
						break
					}
					conflicts.Add(1)
					time.Sleep(time.Duration(min(attempt+1, 10)) * 10 * time.Millisecond)
				}
				if err != nil {
					t.Logf("tool call failed after retries: %v", err)
					failures.Add(1)
				}
				mu.Lock()
				latencies = append(latencies, time.Since(callStart).Seconds())
				mu.Unlock()
			}
		}()
	}
	initialized.Wait()
	close(begin)
	wg.Wait()
	cancel()
	background.Wait()
	elapsed := time.Since(start)
	if failures.Load() != 0 {
		t.Fatalf("mixed workload failures: %d", failures.Load())
	}
	pending, err := audit.Pending(e.ctx, e.pool)
	if err != nil || pending.Incomplete != 0 {
		t.Fatalf("incomplete durable calls: %+v %v", pending, err)
	}
	percentile := func(values []float64, p float64) float64 {
		sort.Float64s(values)
		return values[int(float64(len(values)-1)*p)] * 1000
	}
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	stats := e.pool.Stat()
	report := map[string]any{"replicas": 1, "workers": 8, "calls": len(latencies), "session_creations": len(initializations), "call_p95_ms": percentile(latencies, .95), "call_p99_ms": percentile(latencies, .99), "session_p95_ms": percentile(initializations, .95), "elapsed_seconds": elapsed.Seconds(), "go_heap_bytes": memory.HeapAlloc, "pool_max": stats.MaxConns(), "pool_wait_seconds_total": stats.AcquireDuration().Seconds(), "pool_empty_acquires": stats.EmptyAcquireCount(), "snapshot_refresh_ms": e.syncer.RefreshDuration().Seconds() * 1000, "audit_incomplete": pending.Incomplete, "audit_oldest_incomplete_seconds": pending.OldestIncompleteAgeSeconds, "configuration_churn": churn.Load(), "audit_queries": queries.Load(), "client_retries": conflicts.Load(), "go_version": runtime.Version(), "cpus": runtime.NumCPU()}
	encoded, _ := json.Marshal(report)
	t.Log("LOAD_REPORT", string(encoded))
}
