//go:build acceptance

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/mcp"
)

// TestSQLCostAcceptance measures serial calls, never resets shared statistics,
// and only uses the runner's disposable namespace/database.
func TestSQLCostAcceptance(t *testing.T) {
	if os.Getenv("VMCP_RUN_ACCEPTANCE") != "1" || !acceptanceKubernetes() || os.Getenv("VMCP_SQL_COST_ACCEPTANCE") != "1" {
		t.Fatal("isolated Kubernetes SQL-cost opt-in required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	output := os.Getenv("VMCP_ACCEPTANCE_OUTPUT")
	fixture := &acceptanceFixture{tools: 2, rest: true}
	server := httptest.NewUnstartedServer(fixture)
	listener, err := net.Listen("tcp", net.JoinHostPort(os.Getenv("VMCP_ACCEPTANCE_NODE_IP"), "0"))
	if err != nil {
		t.Fatal(err)
	}
	server.Listener = listener
	server.Start()
	defer server.Close()
	pool, database, master, tokens := acceptanceSeed(t, acceptanceProfile{Servers: 1, Tools: 2, Sessions: 1, REST: true}, []*httptest.Server{server})
	if _, err = pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS pg_stat_statements"); err != nil {
		t.Fatal(err)
	}
	node := &acceptanceNode{name: "acceptance-node-1", binary: os.Getenv("VMCP_ACCEPTANCE_BINARY"), database: database, master: master, redis: os.Getenv("VMCP_ACCEPTANCE_REDIS_URL"), output: output}
	node.start(t)
	defer node.kill()
	client := &acceptanceClient{url: node.url + "/mcp/v1/virtual/accept-vs-0000", token: tokens[0], http: &http.Client{Timeout: 12 * time.Second}, server: 0, rest: true}
	for _, item := range []struct {
		method string
		params any
		id     int64
	}{{mcp.MethodInitialize, map[string]any{"protocolVersion": mcp.SupportedVersions[0], "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "sql-cost", "version": "1"}}, -1}, {mcp.MethodInitialized, nil, 0}} {
		if r := client.request(ctx, item.method, item.params, item.id); r.err != nil {
			t.Fatal(r.err)
		}
	}
	tracker := &acceptanceTracker{}
	var callMS []float64
	call := func() {
		t.Helper()
		began := time.Now()
		r, valid := tracker.call(ctx, client, 0)
		callMS = append(callMS, float64(time.Since(began).Nanoseconds())/1e6)
		if r.err != nil || !valid {
			t.Fatalf("serial call failed: %v valid=%v", r.err, valid)
		}
	}
	call() // Warm the same path before collecting differences.
	snapshot := func(name string) {
		t.Helper()
		var raw []byte
		err := pool.QueryRow(ctx, `SELECT COALESCE(json_agg(s),'[]'::json) FROM (SELECT queryid::text,toplevel,query,calls,rows,total_exec_time,wal_records,wal_bytes::text FROM pg_stat_statements WHERE dbid=(SELECT oid FROM pg_database WHERE datname=current_database()) AND query NOT LIKE '%pg_stat_statements%' ORDER BY queryid,toplevel) s`).Scan(&raw)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(output, name+".json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	snapshot("sql-idle-before")
	time.Sleep(5 * time.Second)
	snapshot("sql-idle-after")
	start := time.Now()
	snapshot("sql-single-before")
	call()
	time.Sleep(100 * time.Millisecond)
	snapshot("sql-single-after")
	singleSeconds := time.Since(start).Seconds()
	snapshot("sql-serial-before")
	for range 20 {
		call()
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	snapshot("sql-serial-after")
	consistency, err := acceptanceCheckAudit(ctx, pool, tracker, fixture)
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]any{"idle_window_seconds": 5, "call_latency_ms": callMS, "serial_calls": 20, "single_calls": 1, "warmup_calls": 1, "single_window_seconds": singleSeconds, "total_measurement_seconds": time.Since(start).Seconds(), "consistency": consistency, "scope": "one initialized session, short REST tool, PII enabled, strict metadata audit, direct dedicated NodePort; includes background SQL; no capacity claim"}
	raw, _ := json.MarshalIndent(result, "", "  ")
	if err = os.WriteFile(filepath.Join(output, "sql-cost.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log(fmt.Sprintf("SQL cost fixture: %d validated calls", 22))
}
