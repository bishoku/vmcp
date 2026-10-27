//go:build acceptance

package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
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
	MemoryUsageBytes   int64
	ReportedCPUPercent float64
	Readiness          json.RawMessage `json:"readiness,omitempty"`
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
			var kubeSamples map[string]acceptanceNodeSample
			if acceptanceKubernetes() {
				kubeSamples = acceptanceKubeSamples(monitorCtx)
			}
			sample := acceptanceSample{Seconds: time.Since(start).Seconds(), Phase: phase.Load().(string), Database: map[string]any{}}
			for name, container := range kubeSamples {
				if !strings.HasPrefix(name, "acceptance-node-") {
					sample.Containers = append(sample.Containers, map[string]any{"name": name, "rss_bytes": container.RSSBytes, "cpu_percent": container.ReportedCPUPercent})
				}
			}
			for _, node := range nodes {
				pid, alive := node.process()
				item := acceptanceNodeSample{Name: node.name, PID: pid, Alive: alive}
				if alive {
					if acceptanceKubernetes() {
						if measured, ok := kubeSamples[node.name]; ok {
							item.RSSBytes = measured.RSSBytes
							item.MemoryUsageBytes = measured.MemoryUsageBytes
							item.ReportedCPUPercent = measured.ReportedCPUPercent
						} else {
							item.Error = "kubelet RSS sample unavailable"
						}
					} else {
						ps, _ := exec.CommandContext(monitorCtx, "ps", "-o", "rss=,%cpu=", "-p", strconv.Itoa(pid)).Output()
						fields := strings.Fields(string(ps))
						if len(fields) >= 2 {
							rss, _ := strconv.ParseInt(fields[0], 10, 64)
							item.RSSBytes = rss * 1024
							item.ReportedCPUPercent, _ = strconv.ParseFloat(fields[1], 64)
						}
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
							if os.Getenv("VMCP_ACCEPTANCE_DIAGNOSTICS") == "1" {
								_ = os.WriteFile(filepath.Join(os.Getenv("VMCP_ACCEPTANCE_OUTPUT"), fmt.Sprintf("metrics-%s-%06d.prom", node.name, int(sample.Seconds))), raw, 0600)
								if ready, err := client.Get(node.url + "/readyz"); err == nil {
									data, _ := io.ReadAll(io.LimitReader(ready.Body, 16384))
									ready.Body.Close()
									if json.Valid(data) {
										item.Readiness = data
									}
								}
							}
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
	names := map[string]bool{}
	for _, sample := range samples {
		for _, node := range sample.Nodes {
			names[node.Name] = true
		}
	}
	for name := range names {
		var rss, cpu, usage []float64
		var steadyRSS []float64
		maxPool, waitStart, waitEnd, cancelled, overflow := float64(0), float64(-1), float64(0), float64(0), float64(0)
		var soakRSS []float64
		for _, sample := range samples {
			for _, node := range sample.Nodes {
				if node.Name != name || !node.Alive {
					continue
				}
				rss = append(rss, float64(node.RSSBytes))
				usage = append(usage, float64(node.MemoryUsageBytes))
				if sample.Phase == "steady" {
					steadyRSS = append(steadyRSS, float64(node.RSSBytes))
				}
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
		sort.Float64s(usage)
		sort.Float64s(rss)
		sort.Float64s(cpu)
		item := map[string]any{"max_rss_bytes": acceptanceQuantile(rss, 1), "median_rss_bytes": acceptanceQuantile(rss, .5), "max_os_reported_cpu_percent": acceptanceQuantile(cpu, 1), "median_os_reported_cpu_percent": acceptanceQuantile(cpu, .5), "max_pool_acquired": maxPool, "max_pool_cancelled_acquires": cancelled, "pool_acquire_seconds_observed": waitEnd - max(waitStart, 0), "max_metric_label_overflows": overflow}
		item["max_container_usage_bytes"] = acceptanceQuantile(usage, 1)
		item["steady_rss_growth_percent"] = acceptanceRSSGrowth(steadyRSS)
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
		t.Fatal("acceptance fault drill requires explicit opt-in; run scripts/acceptance-core.sh")
	}
	binary := os.Getenv("VMCP_ACCEPTANCE_BINARY")
	output := os.Getenv("VMCP_ACCEPTANCE_OUTPUT")
	if binary == "" || output == "" {
		t.Fatal("acceptance binary and output directory required")
	}
	if err := os.MkdirAll(output, 0700); err != nil {
		t.Fatal(err)
	}
	profile := acceptanceProfile{Servers: acceptanceNumber("VMCP_ACCEPTANCE_SERVERS", 300, 1, 1000), Tools: acceptanceNumber("VMCP_ACCEPTANCE_TOOLS", 300, 1, 1000), Resources: acceptanceNumber("VMCP_ACCEPTANCE_RESOURCES", 100, 0, 1000), SteadyRPS: acceptanceNumber("VMCP_ACCEPTANCE_RPS", 200, 1, 2000), BurstRPS: acceptanceNumber("VMCP_ACCEPTANCE_BURST_RPS", 600, 1, 5000), Workers: acceptanceNumber("VMCP_ACCEPTANCE_WORKERS", 128, 1, 2048), Smoke: os.Getenv("VMCP_ACCEPTANCE_SMOKE") == "1"}
	profile.REST = os.Getenv("VMCP_ACCEPTANCE_REST") == "1"
	if profile.REST {
		profile.Tools = acceptanceNumber("VMCP_ACCEPTANCE_TOOLS", 200, 1, 200)
		profile.Resources = 0
	}
	profile.Sessions = profile.Servers * 2
	nodeCount := acceptanceNumber("VMCP_ACCEPTANCE_NODES", 2, 1, 2)
	if nodeCount == 1 && os.Getenv("VMCP_ACCEPTANCE_DATABASE_FOCUS") != "1" {
		t.Fatal("single-node profile requires database focus")
	}
	started := time.Now()
	report := acceptanceReport{StartedUTC: started.UTC().Format(time.RFC3339), Profile: profile, GoVersion: runtime.Version(), HostOS: runtime.GOOS, HostArch: runtime.GOARCH, Functional: map[string]any{}, DataPlaneConfiguration: map[string]any{"nodes": 2, "gomaxprocs_each": 2, "go_soft_memory_limit_each": "768MiB", "db_pool_max_each": 64, "db_acquire_timeout": "2s", "audit": "strict", "redis_failure_policy": "strict", "health_interval": "30s"}, ProxyPolicy: "test round-robin proxy; passive 10s failure cooldown; no request retries; not the production ingress", Thresholds: map[string]any{"steady_success_percent": 99.9, "steady_min_target_completion_percent": 95, "steady_p95_ms": 250, "steady_p99_ms": 1000, "burst_min_target_completion_percent": 80, "burst_min_success_percent": 80, "burst_max_p99_ms": 10000, "fault_phase_min_success_percent": 99, "fault_transition_grace_seconds": 10, "post_fault_max_errors_percent": 0.1, "per_node_max_rss_bytes": int64(1536 << 20), "soak_max_rss_growth_percent": 20}}
	memoryProfile := os.Getenv("VMCP_ACCEPTANCE_MEMORY_PROFILE") == "4g6g"
	if memoryProfile {
		if !acceptanceKubernetes() || os.Getenv("VMCP_ACCEPTANCE_GOMEMLIMIT") != "4GiB" || os.Getenv("VMCP_ACCEPTANCE_MEMORY_LIMIT") != "6Gi" || nodeCount != 1 || os.Getenv("VMCP_ACCEPTANCE_CAPACITY_RATES") != "" || os.Getenv("VMCP_ACCEPTANCE_DATABASE_FOCUS") != "1" {
			t.Fatal("4g6g requires Kubernetes, one node, 4GiB Go / 6Gi container, fixed database-focus load")
		}
		report.Thresholds["per_node_max_rss_bytes"] = int64(5 << 30)
		report.Thresholds["per_node_max_container_usage_bytes"] = int64(6<<30) * 9 / 10
		report.Thresholds["steady_min_target_completion_percent"] = 99
		report.Thresholds["steady_max_rss_growth_percent"] = 20
		report.Thresholds["loaded_revocation_max_seconds"] = 5
	}
	source, _ := exec.Command("git", "rev-parse", "HEAD").Output()
	report.SourceRevision = strings.TrimSpace(string(source))
	report.DataPlaneConfiguration["nodes"] = nodeCount
	report.DataPlaneConfiguration["routing"] = os.Getenv("VMCP_ACCEPTANCE_ROUTING")
	report.DataPlaneConfiguration["steady_seconds"] = acceptanceNumber("VMCP_ACCEPTANCE_STEADY_SECONDS", 300, 5, 7200)
	if value := os.Getenv("VMCP_ACCEPTANCE_GOMEMLIMIT"); value != "" {
		report.DataPlaneConfiguration["go_soft_memory_limit_each"] = value
	}
	report.DataPlaneConfiguration["team_ownership"] = os.Getenv("VMCP_ACCEPTANCE_OWNERSHIP") == "1"
	if acceptanceKubernetes() {
		report.DataPlaneConfiguration["platform"] = "MicroK8s Kubernetes"
		report.DataPlaneConfiguration["postgres_limits"] = "none"
		report.DataPlaneConfiguration["namespace"] = os.Getenv("VMCP_ACCEPTANCE_NAMESPACE")
		report.DataPlaneConfiguration["memory_measurement"] = "kubelet container RSS; CPU recent usageNanoCores"
		report.DataPlaneConfiguration["gomaxprocs_each"] = acceptanceNumber("VMCP_ACCEPTANCE_DP_CPUS", 2, 2, 8)
		report.DataPlaneConfiguration["cpu_limit_each"] = fmt.Sprint(max(4, acceptanceNumber("VMCP_ACCEPTANCE_DP_CPUS", 2, 2, 8)))
		report.DataPlaneConfiguration["memory_limit_each"] = "3Gi"
		if value := os.Getenv("VMCP_ACCEPTANCE_MEMORY_LIMIT"); value != "" {
			report.DataPlaneConfiguration["memory_limit_each"] = value
		}
		report.DataPlaneConfiguration["fault"] = "scale one test Deployment to zero, then restore; not a physical Kubernetes node failure"
		report.ProxyPolicy = "same host test round-robin proxy, passive 10s cooldown, no POST retries; real Kubernetes NodePort Services/CNI to data-plane pods; no ingress capacity claim"
		if os.Getenv("VMCP_ACCEPTANCE_ROUTING") == "pod" {
			report.ProxyPolicy = "host round-robin proxy to direct Kubernetes PodIP; passive 10s cooldown; no POST retries; excludes Service/NodePort/ingress capacity"
		}
	}
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
	fixture := &acceptanceFixture{tools: profile.Tools, resources: profile.Resources, rest: profile.REST}
	fixtures := make([]*httptest.Server, 16)
	for i := range fixtures {
		if acceptanceKubernetes() {
			server := httptest.NewUnstartedServer(fixture)
			listener, listenErr := net.Listen("tcp", net.JoinHostPort(os.Getenv("VMCP_ACCEPTANCE_NODE_IP"), "0"))
			if listenErr != nil {
				t.Fatal(listenErr)
			}
			server.Listener = listener
			server.Start()
			fixtures[i] = server
		} else {
			fixtures[i] = httptest.NewServer(fixture)
		}
		defer fixtures[i].Close()
	}
	t.Logf("Seeding %d upstreams, %d virtual servers, %d tools and %d resources", profile.Servers, profile.Servers, profile.Servers*profile.Tools, profile.Servers*profile.Resources)
	pool, database, master, tokens := acceptanceSeed(t, profile, fixtures)
	nodes := []*acceptanceNode{{name: "acceptance-node-1", binary: binary, database: database, master: master, redis: os.Getenv("VMCP_ACCEPTANCE_REDIS_URL"), output: output}, {name: "acceptance-node-2", binary: binary, database: database, master: master, redis: os.Getenv("VMCP_ACCEPTANCE_REDIS_URL"), output: output}}
	nodes = nodes[:nodeCount]
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
		c := &acceptanceClient{url: balancer.URL + fmt.Sprintf("/mcp/v1/virtual/accept-vs-%04d", server), token: tokens[i%64], http: httpClient, server: server, rest: profile.REST}
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
	metadataCheck := "cross_node_session_and_paginated_masked_metadata"
	if nodeCount == 1 {
		metadataCheck = "single_node_session_and_paginated_masked_metadata"
	}
	report.Functional[metadataCheck] = true
	var phase atomic.Value
	phase.Store("setup")
	stopMonitor := acceptanceMonitor(ctx, nodes, pool, &phase, started)
	tracker := &acceptanceTracker{}
	run := func(name string, rps int, seconds int, fault func(), faultSecond int) {
		phase.Store(name)
		t.Logf("Phase %s: %d req/s for %ds", name, rps, seconds)
		result := acceptanceRunPhase(ctx, name, rps, time.Duration(seconds)*time.Second, clients, profile.Workers, profile.Tools, tracker, fault, time.Duration(faultSecond)*time.Second)
		acceptanceSummarizeSLO(&result)
		if memoryProfile && name == "steady" && result.CompletedRPS < float64(rps)*.99 {
			result.SLOPass = false
		}
		report.Phases = append(report.Phases, result)
		t.Logf("%s: completed %.1f/s, success %.3f%%, p95 %.1fms, p99 %.1fms, errors %d, dropped %d, pass=%t", name, result.CompletedRPS, result.SuccessPercent, result.P95MS, result.P99MS, result.Errors, result.Dropped, result.SLOPass)
		if result.InvalidResults > 0 {
			report.Failures = append(report.Failures, name+" result validation failed")
		}
		if !profile.Smoke && !result.SLOPass && !strings.HasPrefix(name, "capacity-") && !strings.HasPrefix(name, "refine-") {
			report.Failures = append(report.Failures, name+" capacity/latency SLO failed")
		}
	}
	if os.Getenv("VMCP_ACCEPTANCE_DATABASE_FOCUS") == "1" {
		report.DataPlaneConfiguration["database_focus"] = true
		report.DataPlaneConfiguration["delivery_wakeup"] = "local channel; cross-replica polling; no delivery NOTIFY"
		report.DataPlaneConfiguration["postgres_tuned"] = os.Getenv("VMCP_ACCEPTANCE_POSTGRES_TUNED") == "1"
		report.DataPlaneConfiguration["postgres_limits"] = "none"
		report.DataPlaneConfiguration["fault"] = "none; focused database capacity run"
		// Expired fixtures exercise the 15-minute cleanup path in a five-minute run.
		// Use a separate session so audit verification only sees actual client calls.
		if _, err := pool.Exec(ctx, `INSERT INTO sessions(id,virtual_server,key_slug,protocol_version) VALUES('expired-fixture','accept-vs-0000','','2025-06-18');
INSERT INTO mcp_operations(id,session_id,client_request_id,principal_hash,virtual_server,fence,lease_until,state,created_at)
SELECT gen_random_uuid(),'expired-fixture',i::text,decode('00','hex'),'accept-vs-0000',gen_random_uuid(),now()-interval '20 minutes','done',now()-interval '20 minutes' FROM generate_series(1,100000) i;
INSERT INTO mcp_streams(id,session_id,operation_id,expires_at) SELECT gen_random_uuid(),session_id,id,now()-interval '20 minutes' FROM mcp_operations WHERE session_id='expired-fixture';
ANALYZE mcp_operations; ANALYZE mcp_streams;`); err != nil {
			t.Fatal(err)
		}
		report.DataPlaneConfiguration["expired_fixture_operations"] = 100000
		run("warmup", 50, 15, nil, 0)
		var pollProbe func() (map[string]any, error)
		if os.Getenv("VMCP_ACCEPTANCE_POLL_PROBE") == "1" {
			pollProbe = acceptancePollingProbe(ctx, nodes[len(nodes)-1], clients[len(clients)-1], pool, master)
		}
		if raw := os.Getenv("VMCP_ACCEPTANCE_CAPACITY_RATES"); raw != "" {
			rates, rateErr := acceptanceCapacityRates(raw)
			if rateErr != nil {
				t.Fatal(rateErr)
			}
			result := acceptanceCapacitySearch(rates, acceptanceNumber("VMCP_ACCEPTANCE_CAPACITY_SECONDS", 60, 30, 300), func(name string, rps, seconds int) acceptancePhase {
				run(name, rps, seconds, nil, 0)
				return report.Phases[len(report.Phases)-1]
			})
			report.DataPlaneConfiguration["capacity_search"] = result
			if result.PassingRPS > 0 {
				run("recovery", 100, 30, nil, 0)
				run("steady", result.PassingRPS, acceptanceNumber("VMCP_ACCEPTANCE_STEADY_SECONDS", 600, 30, 7200), nil, 0)
			} else {
				report.Failures = append(report.Failures, "no passing capacity stage")
			}
		} else {
			seconds := acceptanceNumber("VMCP_ACCEPTANCE_STEADY_SECONDS", 300, 5, 7200)
			if memoryProfile {
				probe := acceptanceLoadedRevocationClient(t, ctx, nodes[0], tokens[64], httpClient)
				var evidence map[string]any
				var revokeErr error
				run("steady", profile.SteadyRPS, seconds, func() {
					t.Log("loaded key revocation: starting dedicated-key probe")
					evidence, revokeErr = acceptanceLoadedRevocation(ctx, pool, nodes[0], probe)
					t.Logf("loaded key revocation: first401=%v healthy=%v stable=%v error=%v", evidence["first_401_seconds"], evidence["readiness_healthy"], evidence["rejection_stable"], revokeErr)
				}, seconds/2)
				report.DataPlaneConfiguration["loaded_revocation"] = evidence
				report.Functional["key_revocation_under_load_within_5s"] = revokeErr == nil
				if revokeErr != nil {
					report.Failures = append(report.Failures, revokeErr.Error())
				}
			} else {
				run("steady", profile.SteadyRPS, seconds, nil, 0)
			}
		}
		if pollProbe != nil {
			values, probeErr := pollProbe()
			report.DataPlaneConfiguration["remote_poll_probe"] = values
			report.Functional["cross_replica_polling_under_500ms"] = probeErr == nil
			if probeErr != nil {
				report.Failures = append(report.Failures, probeErr.Error())
			}
		}
	} else if profile.Smoke {
		run("warmup", 10, 2, nil, 0)
		run("steady", profile.SteadyRPS, 3, nil, 0)
		run("spike", profile.BurstRPS, 2, nil, 0)
		run("recovery", profile.SteadyRPS, 2, nil, 0)
		run("node-loss", profile.SteadyRPS, 10, nodes[0].kill, 2)
		nodes[0].start(t)
		run("node-rejoin", profile.SteadyRPS, 5, nil, 0)
		run("soak", 10, 5, nil, 0)
	} else {
		run("warmup", 50, acceptanceNumber("VMCP_ACCEPTANCE_WARMUP_SECONDS", 20, 1, 3600), nil, 0)
		if acceptanceKubernetes() {
			run("baseline", 200, 90, nil, 0)
		}
		run("steady", profile.SteadyRPS, acceptanceNumber("VMCP_ACCEPTANCE_STEADY_SECONDS", 90, 1, 3600), nil, 0)
		run("spike", profile.BurstRPS, acceptanceNumber("VMCP_ACCEPTANCE_SPIKE_SECONDS", 20, 1, 600), nil, 0)
		run("recovery", profile.SteadyRPS, 45, nil, 0)
		if profile.REST {
			run("config-refresh", profile.SteadyRPS, 30, func() {
				began := time.Now()
				refreshCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
				defer cancel()
				_, updateErr := pool.Exec(refreshCtx, `UPDATE virtual_servers SET name=name||' refreshed' WHERE slug='accept-vs-0000'`)
				var revision int64
				if updateErr == nil {
					updateErr = pool.QueryRow(refreshCtx, `SELECT revision FROM config_revision`).Scan(&revision)
				}
				ok := false
				for updateErr == nil && refreshCtx.Err() == nil {
					ok = true
					for _, node := range nodes {
						resp, err := httpClient.Get(node.url + "/metrics")
						if err != nil {
							ok = false
							break
						}
						raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
						resp.Body.Close()
						if err != nil || acceptanceMetricValues(string(raw))["vmcp_snapshot_revision"] < float64(revision) {
							ok = false
						}
					}
					if ok {
						break
					}
					time.Sleep(100 * time.Millisecond)
				}
				report.Functional["configuration_refresh_under_rest_load"] = ok
				report.DataPlaneConfiguration["rest_refresh_ms"] = time.Since(began).Milliseconds()
				if !ok {
					report.Failures = append(report.Failures, "REST configuration refresh did not reach both nodes")
				}
			}, 5)
		}
		run("node-loss", profile.SteadyRPS, 90, nodes[0].kill, 20)
		nodes[0].start(t)
		run("node-rejoin", profile.SteadyRPS, 60, nil, 0)
		run("soak", acceptanceNumber("VMCP_ACCEPTANCE_SOAK_RPS", max(1, profile.SteadyRPS/2), 1, 2000), acceptanceNumber("VMCP_ACCEPTANCE_SOAK_SECONDS", 300, 5, 7200), nil, 0)
	}
	phase.Store("verification")
	report.Samples = stopMonitor()
	report.Resources = acceptanceResources(report.Samples)
	if acceptanceKubernetes() {
		for _, node := range nodes {
			var measured bool
			for _, sample := range report.Samples {
				for _, value := range sample.Nodes {
					if value.Name == node.name && value.RSSBytes > 0 {
						measured = true
					}
				}
			}
			if !measured {
				report.Failures = append(report.Failures, "Kubernetes RSS observations missing for "+node.name)
			}
			node.mu.Lock()
			if node.kubeError != "" {
				report.Failures = append(report.Failures, node.kubeError)
			}
			node.mu.Unlock()
		}
	}
	auditCtx, auditCancel := context.WithTimeout(ctx, time.Duration(acceptanceNumber("VMCP_ACCEPTANCE_AUDIT_CHECK_SECONDS", 30, 30, 600))*time.Second)
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
	if err := pool.QueryRow(ctx, `SELECT revision FROM config_revision`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	t.Logf("key revocation: utc=%s target_revision=%d", time.Now().UTC().Format(time.RFC3339Nano), revision)
	deadline := time.Now().Add(30 * time.Second)
	refreshed := false
	lastRevision := map[string]float64{}
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
			observed := acceptanceMetricValues(string(raw))["vmcp_snapshot_revision"]
			if previous, ok := lastRevision[node.name]; !ok || previous != observed {
				t.Logf("key revocation: node=%s metrics_status=%d observed_revision=%g target_revision=%d", node.name, resp.StatusCode, observed, revision)
				lastRevision[node.name] = observed
			}
			if observed < float64(revision) {
				refreshed = false
			}
		}
		if refreshed {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	rejected := true
	for _, node := range nodes {
		for _, path := range []string{"/metrics", "/readyz"} {
			resp, getErr := httpClient.Get(node.url + path)
			if getErr == nil {
				raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
				resp.Body.Close()
				_ = os.WriteFile(filepath.Join(output, "revocation-"+node.name+strings.ReplaceAll(path, "/", "-")+".txt"), raw, 0600)
				t.Logf("key revocation: node=%s path=%s status=%d", node.name, path, resp.StatusCode)
			}
		}
		copy := *clients[0]
		copy.url = node.url + fmt.Sprintf("/mcp/v1/virtual/accept-vs-%04d", copy.server)
		response := copy.request(ctx, mcp.MethodToolsList, nil, -5)
		t.Logf("key revocation: node=%s refreshed=%v http_status=%d error=%v", node.name, refreshed, response.status, response.err)
		if response.status != 401 {
			rejected = false
		}
		if !refreshed {
			report.Failures = append(report.Failures, fmt.Sprintf("snapshot propagation timed out on %s: target=%d observed=%g", node.name, revision, lastRevision[node.name]))
		}
		if response.status != 401 {
			report.Failures = append(report.Failures, fmt.Sprintf("revoked key expected HTTP 401 on %s: status=%d error=%v", node.name, response.status, response.err))
		}
	}
	keyCheck := "key_revocation_propagated_and_rejected_on_both_nodes"
	if nodeCount == 1 {
		keyCheck = "key_revocation_propagated_and_rejected_on_single_node"
	}
	report.Functional[keyCheck] = refreshed && rejected
	if os.Getenv("VMCP_ACCEPTANCE_DIAGNOSTICS") == "1" {
		acceptanceSnapshotDiagnostic(t, ctx, pool, master, output)
	}
	if memoryProfile {
		restartErr := acceptanceKubeNoRestarts(ctx)
		report.Functional["no_data_plane_restart_or_oom"] = restartErr == nil
		if err := restartErr; err != nil {
			report.Failures = append(report.Failures, err.Error())
		}
	}
	if !profile.Smoke {
		for _, value := range report.Resources {
			item := value.(map[string]any)
			if item["max_rss_bytes"].(float64) > float64(report.Thresholds["per_node_max_rss_bytes"].(int64)) {
				report.Failures = append(report.Failures, "node RSS budget exceeded")
			}
			if memoryProfile {
				usage := item["max_container_usage_bytes"].(float64)
				if usage <= 0 || usage > float64(report.Thresholds["per_node_max_container_usage_bytes"].(int64)) {
					report.Failures = append(report.Failures, "container memory usage missing or budget exceeded")
				}
				if item["steady_rss_growth_percent"].(float64) > 20 {
					report.Failures = append(report.Failures, "steady RSS growth requires investigation")
				}
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
