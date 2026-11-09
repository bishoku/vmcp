//go:build acceptance

package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/mcp"
)

// This experiment reports latency, not a pass against a short-upstream SLO.
func TestUpstreamLatencyAcceptance(t *testing.T) {
	if os.Getenv("VMCP_RUN_ACCEPTANCE") != "1" || !acceptanceKubernetes() || os.Getenv("VMCP_UPSTREAM_LATENCY_ACCEPTANCE") != "1" {
		t.Fatal("isolated Kubernetes latency opt-in required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	output := os.Getenv("VMCP_ACCEPTANCE_OUTPUT")
	profile := acceptanceProfile{Servers: 300, Tools: 200, Sessions: 600, SteadyRPS: 100, Workers: 128, REST: true}
	fixture := &acceptanceFixture{tools: profile.Tools, rest: true}
	server := httptest.NewUnstartedServer(fixture)
	listener, err := net.Listen("tcp", net.JoinHostPort(os.Getenv("VMCP_ACCEPTANCE_NODE_IP"), "0"))
	if err != nil {
		t.Fatal(err)
	}
	server.Listener = listener
	server.Start()
	defer server.Close()
	pool, database, master, tokens := acceptanceSeed(t, profile, []*httptest.Server{server})
	if _, err = pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS pg_stat_statements"); err != nil {
		t.Fatal(err)
	}
	node := &acceptanceNode{name: "acceptance-node-1", binary: os.Getenv("VMCP_ACCEPTANCE_BINARY"), database: database, master: master, redis: os.Getenv("VMCP_ACCEPTANCE_REDIS_URL"), output: output}
	node.start(t)
	defer node.kill()
	balancer := acceptanceBalancer([]*acceptanceNode{node})
	defer balancer.Close()
	transport := &http.Transport{MaxIdleConns: 1024, MaxIdleConnsPerHost: 256, IdleConnTimeout: 90 * time.Second}
	defer transport.CloseIdleConnections()
	clientHTTP := &http.Client{Transport: transport, Timeout: 12 * time.Second}
	clients := make([]*acceptanceClient, profile.Sessions)
	for i := range clients {
		idx := i % profile.Servers
		c := &acceptanceClient{url: balancer.URL + fmt.Sprintf("/mcp/v1/virtual/accept-vs-%04d", idx), token: tokens[i%64], http: clientHTTP, server: idx, rest: true}
		if r := c.request(ctx, mcp.MethodInitialize, map[string]any{"protocolVersion": mcp.SupportedVersions[0], "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "latency-diagnostic", "version": "1"}}, -1); r.err != nil {
			t.Fatal(r.err)
		}
		if r := c.request(ctx, mcp.MethodInitialized, nil, 0); r.err != nil {
			t.Fatal(r.err)
		}
		clients[i] = c
	}
	started := time.Now()
	var phase atomic.Value
	phase.Store("setup")
	stopMonitor := acceptanceMonitor(ctx, []*acceptanceNode{node}, pool, &phase, started)
	binary, err := os.ReadFile(node.binary)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(binary)
	report := map[string]any{"profile": profile, "binary_sha256": hex.EncodeToString(sum[:]), "started_utc": started.UTC(), "scope": "observational 100 TPS distributed traffic; strict metadata audit, REST upstream, no faults/config changes; no latency SLO assessed"}
	write := func(name string, value any) {
		t.Helper()
		raw, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(output, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		samples := stopMonitor()
		report["samples"] = samples
		report["resources"] = acceptanceResources(samples)
		report["measurement_completed"] = !t.Failed()
		write("upstream-latency.json", report)
	}()
	snapshot := func(name string) {
		t.Helper()
		var raw json.RawMessage
		err := pool.QueryRow(ctx, `SELECT COALESCE(json_agg(s),'[]'::json) FROM (SELECT queryid::text,toplevel,query,calls,rows,total_exec_time,wal_records,wal_bytes::text FROM pg_stat_statements WHERE dbid=(SELECT oid FROM pg_database WHERE datname=current_database()) AND query NOT LIKE '%pg_stat_statements%' ORDER BY queryid,toplevel) s`).Scan(&raw)
		if err != nil {
			t.Fatal(err)
		}
		write(name+".json", raw)
	}
	tracker := &acceptanceTracker{}
	var stages []map[string]any
	for _, stage := range []struct {
		name           string
		delay, seconds int
	}{{"warmup", 5, 10}, {"baseline", 5, 60}, {"delay400", 400, 120}, {"delay500", 500, 120}, {"recovery", 5, 60}} {
		if fixture.waiting.Load() != 0 {
			t.Fatal("previous phase still has upstream calls")
		}
		fixture.delayMS.Store(int64(stage.delay))
		fixture.maxWaiting.Store(0)
		fixture.waitNanos.Store(0)
		fixture.waitCalls.Store(0)
		phase.Store(stage.name)
		snapshot(stage.name + "-sql-before")
		t.Logf("Phase %s: 100 TPS for %ds, upstream delay %dms", stage.name, stage.seconds, stage.delay)
		result := acceptanceRunPhase(ctx, stage.name, 100, time.Duration(stage.seconds)*time.Second, clients, profile.Workers, profile.Tools, tracker, nil, 0)
		snapshot(stage.name + "-sql-after")
		// Remove the generic harness's unevaluated SLO flag from this diagnostic.
		raw, _ := json.Marshal(result)
		var metrics map[string]any
		_ = json.Unmarshal(raw, &metrics)
		delete(metrics, "slo_pass")
		calls := fixture.waitCalls.Load()
		mean := float64(0)
		if calls > 0 {
			mean = float64(fixture.waitNanos.Load()) / float64(calls) / 1e6
		}
		stages = append(stages, map[string]any{"metrics": metrics, "upstream_delay_ms": stage.delay, "observed_wait_mean_ms": mean, "upstream_calls": calls, "peak_upstream_waiters": fixture.maxWaiting.Load(), "elapsed_seconds_from_start": time.Since(started).Seconds()})
		report["phases"] = stages
		t.Logf("%s: success %.3f%% completed %.2f/s p95 %.2fms p99 %.2fms errors %d dropped %d upstream waiters peak %d", stage.name, result.SuccessPercent, result.CompletedRPS, result.P95MS, result.P99MS, result.Errors, result.Dropped, fixture.maxWaiting.Load())
		if result.InvalidResults > 0 {
			t.Errorf("%s invalid results: %d", stage.name, result.InvalidResults)
		}
	}
	consistency, err := acceptanceCheckAudit(ctx, pool, tracker, fixture)
	report["consistency"] = consistency
	if err != nil {
		t.Fatal(err)
	}
	node.mu.Lock()
	kubeError := node.kubeError
	node.mu.Unlock()
	if kubeError != "" {
		t.Fatal(kubeError)
	}
}

func TestAcceptanceUpstreamWaitCancellation(t *testing.T) {
	f := &acceptanceFixture{}
	f.delayMS.Store(500)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if f.waitUpstream(ctx) {
		t.Fatal("cancelled upstream completed")
	}
	if f.waiting.Load() != 0 || f.waitCalls.Load() != 1 {
		t.Fatal("cancelled waiter leaked")
	}
}
