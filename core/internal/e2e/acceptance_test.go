package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/jackc/pgx/v5/pgxpool"
)

type acceptanceNodeSample struct {
	Name               string
	PID                int
	Alive              bool
	RSSBytes           int64
	ReportedCPUPercent float64
	Metrics            map[string]float64
	Error              string `json:"error,omitempty"`
}
type acceptanceSample struct {
	Seconds    float64
	Phase      string
	Nodes      []acceptanceNodeSample
	Database   map[string]any
	Containers []map[string]any `json:"containers,omitempty"`
}
type acceptanceReport struct {
	StartedUTC                                                string
	SourceRevision, BinarySHA256, GoVersion, HostOS, HostArch string
	Profile                                                   acceptanceProfile
	DataPlaneConfiguration                                    map[string]any
	ProxyPolicy                                               string
	Thresholds                                                map[string]any
	Phases                                                    []acceptancePhase
	Samples                                                   []acceptanceSample
	Resources                                                 map[string]any
	Consistency                                               map[string]any
	Functional                                                map[string]any
	Pass                                                      bool
	Failures                                                  []string
}

func acceptanceMonitor(ctx context.Context, nodes []*acceptanceNode, pool *pgxpool.Pool, phase *atomic.Value, start time.Time) func() []acceptanceSample {
	monitorCtx, cancel := context.WithCancel(ctx)
	var mu sync.Mutex
	var samples []acceptanceSample
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-monitorCtx.Done():
				return
			case <-ticker.C:
			}
			sample := acceptanceSample{Seconds: time.Since(start).Seconds(), Phase: phase.Load().(string), Database: map[string]any{}}
			for _, node := range nodes {
				pid, alive := node.process()
				item := acceptanceNodeSample{Name: node.name, PID: pid, Alive: alive}
				if alive {
					ps, _ := exec.CommandContext(monitorCtx, "ps", "-o", "rss=,%cpu=", "-p", strconv.Itoa(pid)).Output()
					fields := strings.Fields(string(ps))
					if len(fields) >= 2 {
						rss, _ := strconv.ParseInt(fields[0], 10, 64)
						item.RSSBytes = rss * 1024
						item.ReportedCPUPercent, _ = strconv.ParseFloat(fields[1], 64)
					}
					client := &http.Client{Timeout: 2 * time.Second}
					resp, err := client.Get(node.url + "/metrics")
					if err != nil {
						item.Error = "metrics unavailable"
					} else {
						raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
						resp.Body.Close()
						if readErr != nil {
							item.Error = "metrics read failed"
						} else {
							item.Metrics = acceptanceMetricValues(string(raw))
						}
					}
				}
				sample.Nodes = append(sample.Nodes, item)
			}
			queryCtx, queryCancel := context.WithTimeout(monitorCtx, 2*time.Second)
			var commits, rollbacks, inserted, updated, deadlocks, tempBytes, connections, waiting, size int64
			err := pool.QueryRow(queryCtx, `SELECT xact_commit,xact_rollback,tup_inserted,tup_updated,deadlocks,temp_bytes,(SELECT count(*) FROM pg_stat_activity WHERE datname=current_database()),(SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock'),pg_database_size(current_database()) FROM pg_stat_database WHERE datname=current_database()`).Scan(&commits, &rollbacks, &inserted, &updated, &deadlocks, &tempBytes, &connections, &waiting, &size)
			queryCancel()
			if err == nil {
				sample.Database = map[string]any{"commits": commits, "rollbacks": rollbacks, "inserted": inserted, "updated": updated, "deadlocks": deadlocks, "temp_bytes": tempBytes, "connections": connections, "lock_waiters": waiting, "size_bytes": size}
			} else {
				sample.Database["error"] = "database sample unavailable"
			}
			ids := strings.Fields(os.Getenv("VMCP_ACCEPTANCE_CONTAINERS"))
			if len(ids) > 0 {
				args := append([]string{"stats", "--no-stream", "--format", "{{json .}}"}, ids...)
				statsCtx, statsCancel := context.WithTimeout(monitorCtx, 3*time.Second)
				raw, err := exec.CommandContext(statsCtx, "docker", args...).Output()
				statsCancel()
				if err == nil {
					for _, line := range strings.Split(string(raw), "\n") {
						var item map[string]any
						if json.Unmarshal([]byte(line), &item) == nil {
							sample.Containers = append(sample.Containers, item)
						}
					}
				}
			}
			mu.Lock()
			samples = append(samples, sample)
			mu.Unlock()
		}
	}()
	return func() []acceptanceSample {
		cancel()
		<-done
		mu.Lock()
		defer mu.Unlock()
		return append([]acceptanceSample(nil), samples...)
	}
}
func acceptanceResources(samples []acceptanceSample) map[string]any {
	result := map[string]any{}
	for _, name := range []string{"acceptance-node-1", "acceptance-node-2"} {
		var rss, cpu []float64
		maxPool, waitStart, waitEnd, cancelled, overflow := float64(0), float64(-1), float64(0), float64(0), float64(0)
		var soakRSS []float64
		for _, sample := range samples {
			for _, node := range sample.Nodes {
				if node.Name != name || !node.Alive {
					continue
				}
				rss = append(rss, float64(node.RSSBytes))
				cpu = append(cpu, node.ReportedCPUPercent)
				if sample.Phase == "soak" {
					soakRSS = append(soakRSS, float64(node.RSSBytes))
				}
				maxPool = max(maxPool, node.Metrics["vmcp_pool_acquired"])
				if waitStart < 0 {
					waitStart = node.Metrics["vmcp_pool_acquire_seconds_total"]
				}
				waitEnd = max(waitEnd, node.Metrics["vmcp_pool_acquire_seconds_total"])
				cancelled = max(cancelled, node.Metrics["vmcp_pool_cancelled_acquires_total"])
				overflow = max(overflow, node.Metrics["vmcp_operation_label_overflows_total"])
			}
		}
		sort.Float64s(rss)
		sort.Float64s(cpu)
		item := map[string]any{"max_rss_bytes": acceptanceQuantile(rss, 1), "median_rss_bytes": acceptanceQuantile(rss, .5), "max_os_reported_cpu_percent": acceptanceQuantile(cpu, 1), "median_os_reported_cpu_percent": acceptanceQuantile(cpu, .5), "max_pool_acquired": maxPool, "max_pool_cancelled_acquires": cancelled, "pool_acquire_seconds_observed": waitEnd - max(waitStart, 0), "max_metric_label_overflows": overflow}
		if len(soakRSS) >= 6 {
			part := len(soakRSS) / 3
			first := append([]float64(nil), soakRSS[:part]...)
			last := append([]float64(nil), soakRSS[len(soakRSS)-part:]...)
			sort.Float64s(first)
			sort.Float64s(last)
			begin, end := acceptanceQuantile(first, .5), acceptanceQuantile(last, .5)
			item["soak_first_third_median_rss_bytes"] = begin
			item["soak_last_third_median_rss_bytes"] = end
			if begin > 0 {
				item["soak_rss_growth_percent"] = (end - begin) * 100 / begin
			}
		}
		result[name] = item
	}
	return result
}

func TestScaleAcceptance(t *testing.T) {
	if os.Getenv("VMCP_RUN_ACCEPTANCE") != "1" {
		t.Skip("opt-in destructive fault drill against only its own child processes")
	}
	binary := os.Getenv("VMCP_ACCEPTANCE_BINARY")
	output := os.Getenv("VMCP_ACCEPTANCE_OUTPUT")
	if binary == "" || output == "" {
		t.Fatal("acceptance binary and output directory required")
	}
	if err := os.MkdirAll(output, 0700); err != nil {
		t.Fatal(err)
	}
	profile := acceptanceProfile{Servers: acceptanceNumber("VMCP_ACCEPTANCE_SERVERS", 300, 1, 1000), Tools: acceptanceNumber("VMCP_ACCEPTANCE_TOOLS", 300, 1, 1000), Resources: acceptanceNumber("VMCP_ACCEPTANCE_RESOURCES", 100, 0, 1000), SteadyRPS: acceptanceNumber("VMCP_ACCEPTANCE_RPS", 200, 1, 2000), BurstRPS: acceptanceNumber("VMCP_ACCEPTANCE_BURST_RPS", 600, 1, 5000), Workers: 128, Smoke: os.Getenv("VMCP_ACCEPTANCE_SMOKE") == "1"}
	profile.Sessions = profile.Servers * 2
	started := time.Now()
	report := acceptanceReport{StartedUTC: started.UTC().Format(time.RFC3339), Profile: profile, GoVersion: runtime.Version(), HostOS: runtime.GOOS, HostArch: runtime.GOARCH, Functional: map[string]any{}, DataPlaneConfiguration: map[string]any{"nodes": 2, "gomaxprocs_each": 2, "go_soft_memory_limit_each": "768MiB", "db_pool_max_each": 64, "db_acquire_timeout": "2s", "audit": "strict", "redis_failure_policy": "strict", "health_interval": "30s"}, ProxyPolicy: "test round-robin proxy; passive 10s failure cooldown; no request retries; not the production ingress", Thresholds: map[string]any{"steady_success_percent": 99.9, "steady_min_target_completion_percent": 95, "steady_p95_ms": 250, "steady_p99_ms": 1000, "burst_min_target_completion_percent": 80, "burst_min_success_percent": 80, "burst_max_p99_ms": 10000, "fault_phase_min_success_percent": 99, "fault_transition_grace_seconds": 10, "post_fault_max_errors_percent": 0.1, "per_node_max_rss_bytes": int64(1536 << 20), "soak_max_rss_growth_percent": 20}}
	source, _ := exec.Command("git", "rev-parse", "HEAD").Output()
	report.SourceRevision = strings.TrimSpace(string(source))
	file, err := os.Open(binary)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	_, _ = io.Copy(hash, file)
	file.Close()
	report.BinarySHA256 = hex.EncodeToString(hash.Sum(nil))
	defer func() {
		report.Pass = len(report.Failures) == 0 && !t.Failed()
		raw, _ := json.MarshalIndent(report, "", "  ")
		if err := os.WriteFile(filepath.Join(output, "report.json"), raw, 0600); err != nil {
			t.Error(err)
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fixture := &acceptanceFixture{tools: profile.Tools, resources: profile.Resources}
	fixtures := make([]*httptest.Server, 16)
	for i := range fixtures {
		fixtures[i] = httptest.NewServer(fixture)
		defer fixtures[i].Close()
	}
	t.Logf("Seeding %d upstreams, %d virtual servers, %d tools and %d resources", profile.Servers, profile.Servers, profile.Servers*profile.Tools, profile.Servers*profile.Resources)
	pool, database, master, tokens := acceptanceSeed(t, profile, fixtures)
	nodes := []*acceptanceNode{{name: "acceptance-node-1", binary: binary, database: database, master: master, redis: os.Getenv("VMCP_ACCEPTANCE_REDIS_URL"), output: output}, {name: "acceptance-node-2", binary: binary, database: database, master: master, redis: os.Getenv("VMCP_ACCEPTANCE_REDIS_URL"), output: output}}
	for _, node := range nodes {
		node.start(t)
		defer node.kill()
	}
	balancer := acceptanceBalancer(nodes)
	defer balancer.Close()
	transport := &http.Transport{MaxIdleConns: 1024, MaxIdleConnsPerHost: 256, IdleConnTimeout: 90 * time.Second}
	defer transport.CloseIdleConnections()
	httpClient := &http.Client{Transport: transport, Timeout: 12 * time.Second}
	clients := make([]*acceptanceClient, profile.Sessions)
	t.Logf("Initializing %d cross-node persistent sessions", len(clients))
	for i := range clients {
		server := i % profile.Servers
		c := &acceptanceClient{url: balancer.URL + fmt.Sprintf("/mcp/v1/virtual/accept-vs-%04d", server), token: tokens[i%len(tokens)], http: httpClient, server: server}
		if response := c.request(ctx, mcp.MethodInitialize, map[string]any{"protocolVersion": mcp.SupportedVersions[0], "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "acceptance-driver", "version": "1"}}, -1); response.err != nil {
			t.Fatal(response.err)
		}
		if response := c.request(ctx, mcp.MethodInitialized, nil, 0); response.err != nil {
			t.Fatal(response.err)
		}
		clients[i] = c
	}
	// Validate paginated, masked metadata through both real nodes using the same session.
	for _, node := range nodes {
		for i := 0; i < min(10, len(clients)); i++ {
			copy := *clients[i]
			copy.url = node.url + fmt.Sprintf("/mcp/v1/virtual/accept-vs-%04d", copy.server)
			count := 0
			cursor := ""
			for {
				response := copy.request(ctx, mcp.MethodToolsList, map[string]string{"cursor": cursor}, -2)
				if response.err != nil {
					t.Fatal(response.err)
				}
				var page mcp.ListToolsResult
				if err = json.Unmarshal(response.result, &page); err != nil {
					t.Fatal(err)
				}
				for _, tool := range page.Tools {
					if copy.server%10 == 0 && strings.Contains(tool.Description, "support@example.com") {
						t.Fatal("metadata PII leak")
					}
				}
				count += len(page.Tools)
				if page.NextCursor == "" {
					break
				}
				cursor = page.NextCursor
			}
			if count != profile.Tools {
				t.Fatal("catalog pagination count changed")
			}
		}
	}
	report.Functional["cross_node_session_and_paginated_masked_metadata"] = true
	var phase atomic.Value
	phase.Store("setup")
	stopMonitor := acceptanceMonitor(ctx, nodes, pool, &phase, started)
	tracker := &acceptanceTracker{}
	run := func(name string, rps int, seconds int, fault func(), faultSecond int) {
		phase.Store(name)
		t.Logf("Phase %s: %d req/s for %ds", name, rps, seconds)
		result := acceptanceRunPhase(ctx, name, rps, time.Duration(seconds)*time.Second, clients, profile.Workers, profile.Tools, tracker, fault, time.Duration(faultSecond)*time.Second)
		acceptanceSummarizeSLO(&result)
		report.Phases = append(report.Phases, result)
		t.Logf("%s: completed %.1f/s, success %.3f%%, p95 %.1fms, p99 %.1fms, errors %d, dropped %d, pass=%t", name, result.CompletedRPS, result.SuccessPercent, result.P95MS, result.P99MS, result.Errors, result.Dropped, result.SLOPass)
		if result.InvalidResults > 0 {
			report.Failures = append(report.Failures, name+" result validation failed")
		}
		if !profile.Smoke && !result.SLOPass {
			report.Failures = append(report.Failures, name+" capacity/latency SLO failed")
		}
	}
	if profile.Smoke {
		run("warmup", 10, 2, nil, 0)
		run("steady", profile.SteadyRPS, 3, nil, 0)
		run("spike", profile.BurstRPS, 2, nil, 0)
		run("recovery", profile.SteadyRPS, 2, nil, 0)
		run("node-loss", profile.SteadyRPS, 10, nodes[0].kill, 2)
		nodes[0].start(t)
		run("node-rejoin", profile.SteadyRPS, 5, nil, 0)
		run("soak", 10, 5, nil, 0)
	} else {
		run("warmup", 50, 20, nil, 0)
		run("steady", profile.SteadyRPS, 90, nil, 0)
		run("spike", profile.BurstRPS, 20, nil, 0)
		run("recovery", profile.SteadyRPS, 45, nil, 0)
		run("node-loss", profile.SteadyRPS, 90, nodes[0].kill, 20)
		nodes[0].start(t)
		run("node-rejoin", profile.SteadyRPS, 60, nil, 0)
		run("soak", max(1, profile.SteadyRPS/2), 300, nil, 0)
	}
	phase.Store("verification")
	report.Samples = stopMonitor()
	report.Resources = acceptanceResources(report.Samples)
	auditCtx, auditCancel := context.WithTimeout(ctx, 30*time.Second)
	consistency, err := acceptanceCheckAudit(auditCtx, pool, tracker, fixture)
	auditCancel()
	report.Consistency = consistency
	if consistency != nil && (consistency["active_virtual_servers"] != profile.Servers || consistency["active_upstreams"] != profile.Servers) {
		report.Failures = append(report.Failures, "traffic did not cover the full server inventory")
	}
	if err != nil {
		report.Failures = append(report.Failures, err.Error())
	}
	invalidKeyOK := true
	for _, node := range nodes {
		copy := *clients[len(clients)-1]
		copy.url = node.url + fmt.Sprintf("/mcp/v1/virtual/accept-vs-%04d", copy.server)
		copy.token = "invalid-synthetic-key"
		response := copy.request(ctx, mcp.MethodInitialize, map[string]any{}, -3)
		if response.status != 401 {
			invalidKeyOK = false
			report.Failures = append(report.Failures, "invalid key was not rejected")
		}
	}
	report.Functional["invalid_keys_rejected_on_both_nodes"] = invalidKeyOK
	// DELETE must terminate a shared session immediately on both nodes.
	sessionOK := true
	closed := clients[min(1, len(clients)-1)]
	req, _ := http.NewRequestWithContext(ctx, "DELETE", closed.url, nil)
	req.Header.Set("Authorization", "Bearer "+closed.token)
	req.Header.Set("Mcp-Session-Id", closed.session)
	req.Header.Set("MCP-Protocol-Version", mcp.SupportedVersions[0])
	resp, err := httpClient.Do(req)
	if err != nil {
		sessionOK = false
		report.Failures = append(report.Failures, "session DELETE failed")
	} else {
		resp.Body.Close()
		if resp.StatusCode != 204 {
			sessionOK = false
			report.Failures = append(report.Failures, "session DELETE status changed")
		}
	}
	for _, node := range nodes {
		copy := *closed
		copy.url = node.url + fmt.Sprintf("/mcp/v1/virtual/accept-vs-%04d", copy.server)
		if response := copy.request(ctx, mcp.MethodToolsList, nil, -4); response.status != 404 {
			sessionOK = false
			report.Failures = append(report.Failures, "terminated session accepted on "+node.name)
		}
	}
	report.Functional["session_termination_authoritative_on_both_nodes"] = sessionOK
	if _, err = pool.Exec(ctx, `UPDATE agent_keys SET revoked_at=now() WHERE slug='accept-key-00'`); err != nil {
		t.Fatal(err)
	}
	var revision int64
	_ = pool.QueryRow(ctx, `SELECT revision FROM config_revision`).Scan(&revision)
	deadline := time.Now().Add(30 * time.Second)
	refreshed := false
	for time.Now().Before(deadline) {
		refreshed = true
		for _, node := range nodes {
			resp, err := httpClient.Get(node.url + "/metrics")
			if err != nil {
				refreshed = false
				break
			}
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
			resp.Body.Close()
			if acceptanceMetricValues(string(raw))["vmcp_snapshot_revision"] < float64(revision) {
				refreshed = false
			}
		}
		if refreshed {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	for _, node := range nodes {
		copy := *clients[0]
		copy.url = node.url + fmt.Sprintf("/mcp/v1/virtual/accept-vs-%04d", copy.server)
		if response := copy.request(ctx, mcp.MethodToolsList, nil, -5); response.status != 401 || !refreshed {
			report.Failures = append(report.Failures, "revoked key accepted or snapshot propagation timed out on "+node.name)
		}
	}
	report.Functional["key_revocation_propagated_and_rejected_on_both_nodes"] = refreshed
	if !profile.Smoke {
		for _, value := range report.Resources {
			item := value.(map[string]any)
			if item["max_rss_bytes"].(float64) > float64(1536<<20) {
				report.Failures = append(report.Failures, "node RSS budget exceeded")
			}
			if growth, ok := item["soak_rss_growth_percent"].(float64); ok && growth > 20 {
				report.Failures = append(report.Failures, "soak RSS growth requires investigation")
			}
		}
	}
	if len(report.Failures) > 0 {
		t.Errorf("acceptance findings: %s", strings.Join(report.Failures, "; "))
	}
}
