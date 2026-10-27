//go:build acceptance

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/omni"
)

// TestOmniAcceptance uses the existing isolated Kubernetes deployment, fixture,
// open-loop generator and metrics sampler. Nothing targets an existing namespace.
func TestOmniAcceptance(t *testing.T) {
	if os.Getenv("VMCP_RUN_ACCEPTANCE") != "1" || !acceptanceKubernetes() {
		t.Skip("isolated Kubernetes acceptance only")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output := os.Getenv("VMCP_ACCEPTANCE_OUTPUT")
	functionalOnly := os.Getenv("VMCP_OMNI_FUNCTIONAL_ONLY") == "1"
	seconds := acceptanceNumber("VMCP_OMNI_ACCEPTANCE_SECONDS", 600, 30, 1800)
	if functionalOnly {
		seconds = 30
	}
	report := map[string]any{"functional_only": functionalOnly, "tools": 1000, "servers": 10, "target_tps": 100, "seconds_per_phase": seconds, "routing": "NodePort/CNI; synthetic upstreams on host", "omni_resources": "2 CPU, GOMEMLIMIT=512MiB, hard limit=1Gi", "workload": "50% REST, 50% native MCP; strict audit; separate 100 TPS discovery interference phase"}
	if functionalOnly {
		report["workload"] = "100 TPS across nine unaffected vMCPs: four REST and five native MCP; grant and publication revocation"
	}
	defer func() {
		report["pass"] = !t.Failed()
		b, _ := json.MarshalIndent(report, "", "  ")
		_ = os.WriteFile(filepath.Join(output, "omni-report.json"), b, 0600)
	}()
	rest := &acceptanceFixture{rest: true, tools: 100}
	native := &acceptanceFixture{tools: 100}
	fixture := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var n int
		_, _ = fmt.Sscanf(r.URL.Path, "/up-%04d", &n)
		if n%2 == 0 {
			rest.ServeHTTP(w, r)
		} else {
			native.ServeHTTP(w, r)
		}
	}))
	listener, err := net.Listen("tcp", net.JoinHostPort(os.Getenv("VMCP_ACCEPTANCE_NODE_IP"), "0"))
	if err != nil {
		t.Fatal(err)
	}
	fixture.Listener = listener
	fixture.Start()
	defer fixture.Close()
	pool, database, master, tokens := acceptanceSeed(t, acceptanceProfile{Servers: 10, Tools: 100, REST: true}, []*httptest.Server{fixture})
	execSQL := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	execSQL(`UPDATE upstreams SET kind='mcp',encrypted_rest_config=NULL WHERE right(slug,1)::int % 2=1`)
	for i := 0; i < 100; i++ {
		tool := acceptanceTool(i)
		b, _ := json.Marshal(tool)
		execSQL(`UPDATE upstream_tools t SET input_schema=$1::jsonb,definition=$2::jsonb FROM upstreams u WHERE u.id=t.upstream_id AND u.kind='mcp' AND t.name=$3`, string(tool.InputSchema), string(b), tool.Name)
	}
	// Explicitly approved ownership path, including one grant reserved for revocation.
	execSQL(`INSERT INTO teams(slug,name,issuer,group_value) VALUES('omni-provider','Provider','https://test.invalid','providers'),('omni-consumer','Consumer','https://test.invalid','consumers')`)
	execSQL(`INSERT INTO projects(slug,name,owner_team_id) SELECT 'omni-chat','Chat',id FROM teams WHERE slug='omni-consumer'`)
	execSQL(`UPDATE upstreams SET owner_team_id=(SELECT id FROM teams WHERE slug='omni-provider')`)
	execSQL(`UPDATE virtual_servers SET project_id=(SELECT id FROM projects WHERE slug='omni-chat')`)
	execSQL(`INSERT INTO tool_grants(virtual_server_id,upstream_id,tool_name,status,actor) SELECT virtual_server_id,upstream_id,tool_name,'approved','acceptance' FROM virtual_server_tools`)
	var unpublished int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM virtual_servers WHERE omni_enabled`).Scan(&unpublished)
	if unpublished != 0 {
		t.Fatal("publication default is not false")
	}
	node := &acceptanceNode{name: "acceptance-node-1", database: database, master: master, output: output}
	node.start(t)
	defer node.kill()
	transport := &http.Transport{MaxIdleConns: 256, MaxIdleConnsPerHost: 128}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 12 * time.Second}
	var phase atomic.Value
	phase.Store("setup")
	omniNode := &acceptanceNode{name: "omni-mcp"}
	stopMonitor := acceptanceMonitor(ctx, []*acceptanceNode{node, omniNode}, pool, &phase, time.Now())
	defer func() { report["samples"] = stopMonitor() }()
	direct := make([]*acceptanceClient, 20)
	for i := range direct {
		c := &acceptanceClient{url: node.url + fmt.Sprintf("/mcp/v1/virtual/accept-vs-%04d", i%10), token: tokens[i], http: client, server: i % 10, rest: i%2 == 0}
		r := c.request(ctx, mcp.MethodInitialize, map[string]any{"protocolVersion": mcp.SupportedVersions[0], "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "omni-acceptance", "version": "1"}}, -1)
		if r.err != nil {
			t.Fatal(r.err)
		}
		if r = c.request(ctx, mcp.MethodInitialized, nil, 0); r.err != nil {
			t.Fatal(r.err)
		}
		direct[i] = c
	}
	tracker := &acceptanceTracker{}
	phases := []acceptancePhase{}
	report["phases"] = phases
	run := func(name string, clients []*acceptanceClient, fault func()) {
		phase.Store(name)
		t.Logf("%s: 100 TPS for %ds", name, seconds)
		r := acceptanceRunPhase(ctx, name, 100, time.Duration(seconds)*time.Second, clients, 64, 100, tracker, fault, time.Duration(seconds/2)*time.Second)
		acceptanceSummarizeSLO(&r)
		r.SLOPass = r.SLOPass && r.CompletedRPS >= 99
		phases = append(phases, r)
		report["phases"] = phases
		phase.Store("setup") // Exclude index startup/transition from load resource samples.
		t.Logf("%s: %.3f TPS, success %.3f%% p95 %.2fms p99 %.2fms errors=%d drops=%d", name, r.CompletedRPS, r.SuccessPercent, r.P95MS, r.P99MS, r.Errors, r.Dropped)
		if !r.SLOPass {
			t.Errorf("%s failed SLO: %v", name, r.ErrorExamples)
		}
	}
	// First phase has neither published Omni metadata nor an Omni process.
	if !functionalOnly {
		run("direct-baseline", direct, nil)
	}
	execSQL(`UPDATE virtual_servers SET omni_enabled=true`)
	kube := func(args ...string) {
		t.Helper()
		c, done := context.WithTimeout(ctx, 120*time.Second)
		defer done()
		if _, err := acceptanceKubectl(c, nil, args...); err != nil {
			t.Fatal(err)
		}
	}
	kube("scale", "deployment/omni-mcp", "--replicas=1")
	raw, err := acceptanceKubectl(ctx, nil, "get", "service", "omni-mcp", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	var svc struct {
		Spec struct{ Ports []struct{ NodePort int } }
	}
	if json.Unmarshal(raw, &svc) != nil || len(svc.Spec.Ports) != 1 {
		t.Fatal("Omni service missing")
	}
	omniURL := fmt.Sprintf("http://%s:%d", os.Getenv("VMCP_ACCEPTANCE_NODE_IP"), svc.Spec.Ports[0].NodePort)
	wait := func(label string, check func() bool) {
		t.Helper()
		until := time.Now().Add(90 * time.Second)
		for time.Now().Before(until) {
			if check() {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal(label + " timed out")
	}
	wait("Omni index", func() bool {
		r, e := client.Get(omniURL + "/readyz")
		if e != nil {
			return false
		}
		r.Body.Close()
		return r.StatusCode == 200
	})
	omniNode.mu.Lock()
	omniNode.url = omniURL
	omniNode.alive = true
	omniNode.mu.Unlock()
	routed := make([]*acceptanceClient, len(direct))
	for i, c := range direct {
		copy := *c
		copy.url = omniURL + "/mcp/v1/omni"
		copy.session = ""
		copy.omni = true
		routed[i] = &copy
	}
	rpc := func(c *acceptanceClient, name string, args any) acceptanceResponse {
		return c.request(ctx, "tools/call", map[string]any{"name": name, "arguments": args}, -100)
	}
	ref := omni.Ref("accept-vs-0000", "tool-000")
	search := rpc(routed[0], "discover_tools", map[string]any{"query": "tool", "limit": 5})
	if search.err != nil {
		t.Fatal(search.err)
	}
	schema := rpc(routed[0], "get_tool_schema", map[string]any{"tool_ref": ref})
	if schema.err != nil || !strings.Contains(string(schema.result), "schema_version") {
		t.Fatal("schema failed", schema.err)
	}
	// Reserved key is not used by measured traffic. Probe until first 401 and for
	// another ten seconds; failed probes do not count as ordinary traffic errors.
	probe := *routed[0]
	probe.token = tokens[63]
	revoke := func() {
		_, err := pool.Exec(ctx, `UPDATE agent_keys SET revoked_at=now() WHERE slug='accept-key-63'`)
		if err != nil {
			t.Error(err)
			return
		}
		start := time.Now()
		first := time.Duration(0)
		lateSuccess := 0
		for time.Since(start) < 10*time.Second {
			r := rpc(&probe, "get_tool_schema", map[string]any{"tool_ref": ref})
			if r.status == 401 && first == 0 {
				first = time.Since(start)
			}
			if first > 0 && r.status != 401 {
				lateSuccess++
			}
			time.Sleep(50 * time.Millisecond)
		}
		report["loaded_key_revocation"] = map[string]any{"first_401_seconds": first.Seconds(), "responses_other_than_401_after_first": lateSuccess}
		if first == 0 || first > 5*time.Second || lateSuccess != 0 {
			t.Error("loaded key revocation failed")
		}
	}
	if !functionalOnly {
		run("omni-invoke", routed, revoke)
		// Keep direct calls at 100 TPS, add a separately measured 100 search calls/s.
		var searchOK, searchFail, hybrid, fallback atomic.Int64
		var searchMu sync.Mutex
		var searchLatencies []float64
		searchCtx, searchCancel := context.WithCancel(ctx)
		var wg sync.WaitGroup
		wg.Go(func() {
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			slots := make(chan struct{}, 32)
			var workers sync.WaitGroup
			defer workers.Wait()
			for {
				select {
				case <-searchCtx.Done():
					return
				case <-ticker.C:
					select {
					case slots <- struct{}{}:
						workers.Go(func() {
							defer func() { <-slots }()
							began := time.Now()
							r := rpc(routed[0], "discover_tools", map[string]any{"query": "Synthetic tool", "limit": 5})
							if r.err == nil {
								searchOK.Add(1)
								var value struct {
									Structured struct {
										Mode string `json:"search_mode"`
									} `json:"structuredContent"`
								}
								_ = json.Unmarshal(r.result, &value)
								if value.Structured.Mode == "hybrid" {
									hybrid.Add(1)
								} else {
									fallback.Add(1)
								}
								searchMu.Lock()
								searchLatencies = append(searchLatencies, float64(time.Since(began))/float64(time.Millisecond))
								searchMu.Unlock()
							} else {
								searchFail.Add(1)
							}
						})
					default:
						searchFail.Add(1)
					}
				}
			}
		})
		run("direct-with-discovery", direct, nil)
		searchCancel()
		wg.Wait()
		sort.Float64s(searchLatencies)
		report["discovery_interference"] = map[string]any{"success": searchOK.Load(), "errors_or_dropped": searchFail.Load(), "hybrid": hybrid.Load(), "keyword_or_fallback": fallback.Load(), "p95_ms": acceptanceQuantile(searchLatencies, .95), "p99_ms": acceptanceQuantile(searchLatencies, .99)}
		if searchFail.Load() > searchOK.Load()/999 {
			t.Error("discovery success below 99.9%")
		}
	}

	if functionalOnly {
		phase.Store("loaded-governance-revocation")
		unaffected := []*acceptanceClient{}
		for _, c := range routed {
			if c.server != 0 {
				unaffected = append(unaffected, c)
			}
		}
		checkDenied := func(label, reference string) {
			began := time.Now()
			first := time.Duration(0)
			deadline := time.Now().Add(7 * time.Second)
			for time.Now().Before(deadline) {
				r := rpc(routed[0], "get_tool_schema", map[string]any{"tool_ref": reference})
				if r.status == 404 {
					first = time.Since(began)
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
			report[label+"_seconds_under_load"] = first.Seconds()
			if first == 0 || first > 5*time.Second {
				t.Error(label + " propagation exceeded 5s")
				return
			}
			for n := 0; n < 20; n++ {
				r := rpc(routed[0], "invoke_tool", map[string]any{"tool_ref": reference, "arguments": map[string]any{"body": map[string]string{"call_id": fmt.Sprintf("denied-%s-%d", label, n), "message": "must not execute"}}})
				if r.status != 404 {
					t.Errorf("%s invoke after effective revocation returned %d", label, r.status)
				}
				r = rpc(routed[0], "discover_tools", map[string]any{"query": "tool-099", "server_id": "accept-vs-0000", "limit": 5})
				if r.err != nil || strings.Contains(string(r.result), reference) {
					t.Errorf("%s discovery retained revoked reference", label)
				}
				time.Sleep(50 * time.Millisecond)
			}
		}
		fault := func() {
			if _, err := pool.Exec(ctx, `DELETE FROM tool_grants WHERE tool_name='tool-099' AND virtual_server_id=(SELECT id FROM virtual_servers WHERE slug='accept-vs-0000')`); err != nil {
				t.Error(err)
				return
			}
			checkDenied("grant", omni.Ref("accept-vs-0000", "tool-099"))
			if _, err := pool.Exec(ctx, `UPDATE virtual_servers SET omni_enabled=false WHERE slug='accept-vs-0000'`); err != nil {
				t.Error(err)
				return
			}
			checkDenied("publication", ref)
			if r, valid := tracker.call(ctx, direct[0], 1); r.err != nil || !valid {
				t.Error("unpublication changed direct vMCP access", r.err)
			}
		}
		r := acceptanceRunPhase(ctx, "loaded-governance-revocation", 100, 30*time.Second, unaffected, 64, 100, tracker, fault, 5*time.Second)
		acceptanceSummarizeSLO(&r)
		report["loaded_governance_traffic"] = r
		if r.InvalidResults != 0 || r.SuccessPercent < 99 {
			t.Error("governance fault traffic failed", r.ErrorExamples)
		}
	}
	// Preserve original-pod GC evidence before deliberate replacements.
	for _, name := range []string{"acceptance-node-1", "omni-mcp"} {
		if logs, err := acceptanceKubectl(ctx, nil, "logs", "deployment/"+name, "--tail=-1"); err == nil {
			_ = os.WriteFile(filepath.Join(output, name+"-original-pod.log"), logs, 0600)
		} else {
			t.Error(err)
		}
	}
	phase.Store("faults")
	if control := os.Getenv("VMCP_TEST_EMBEDDING_CONTROL_URL"); control != "" {
		response, err := client.Post(control+"/test/fail", "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		r := rpc(routed[0], "discover_tools", map[string]any{"query": "tool", "limit": 5})
		if r.err != nil || !strings.Contains(string(r.result), "keyword-fallback") {
			t.Error("embedding outage did not fall back", r.err)
		}
		report["embedding_outage_keyword_fallback"] = r.err == nil && strings.Contains(string(r.result), "keyword-fallback")
		response, err = client.Post(control+"/test/recover", "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
	}
	// A stale index must never preserve a revoked grant or unpublished server.
	execSQL(`DELETE FROM tool_grants WHERE tool_name='tool-000' AND virtual_server_id=(SELECT id FROM virtual_servers WHERE slug='accept-vs-0000')`)
	began := time.Now()
	wait("grant removal", func() bool { return rpc(routed[0], "get_tool_schema", map[string]any{"tool_ref": ref}).status == 404 })
	report["grant_revoke_seconds"] = time.Since(began).Seconds()
	execSQL(`UPDATE virtual_servers SET omni_enabled=false WHERE slug='accept-vs-0000'`)
	began = time.Now()
	wait("unpublish", func() bool {
		return rpc(routed[0], "get_tool_schema", map[string]any{"tool_ref": omni.Ref("accept-vs-0000", "tool-001")}).status == 404
	})
	report["unpublish_seconds"] = time.Since(began).Seconds()
	kube("scale", "deployment/omni-mcp", "--replicas=0")
	kube("rollout", "status", "deployment/omni-mcp", "--timeout=90s")
	if r, valid := tracker.call(ctx, direct[1], 1); r.err != nil || !valid {
		t.Error("Omni outage affected direct path", r.err)
	}
	kube("scale", "deployment/omni-mcp", "--replicas=1")
	wait("Omni rebuild", func() bool {
		r, e := client.Get(omniURL + "/readyz")
		if e != nil {
			return false
		}
		r.Body.Close()
		return r.StatusCode == 200
	})
	if r, valid := tracker.call(ctx, routed[1], 1); r.err != nil || !valid {
		t.Error("Omni rebuild invocation failed", r.err)
	}
	kube("scale", "deployment/acceptance-node-1", "--replicas=0")
	kube("rollout", "status", "deployment/acceptance-node-1", "--timeout=90s")
	r := rpc(routed[1], "discover_tools", map[string]any{"query": "tool"})
	report["data_plane_outage_http"] = r.status
	if r.status != 503 {
		t.Error("data-plane outage did not fail closed", r.status)
	}
	kube("scale", "deployment/acceptance-node-1", "--replicas=1")
	kube("rollout", "status", "deployment/acceptance-node-1", "--timeout=90s")
	var auditCount, invalidActor int
	if err := pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE actor_id='' OR virtual_server='' OR trace_id='') FROM audit_events WHERE action='omni/tools/call' AND status='ok'`).Scan(&auditCount, &invalidActor); err != nil {
		t.Error(err)
	}
	report["omni_audit"] = map[string]any{"successful_rows": auditCount, "missing_identity_target_or_trace": invalidActor}
	if auditCount == 0 || invalidActor != 0 {
		t.Error("Omni audit identity evidence missing")
	}
	rows, queryErr := pool.Query(ctx, `SELECT status,reason,count(*) FROM audit_events WHERE status IN ('error','denied','tool_error') GROUP BY status,reason`)
	if queryErr != nil {
		t.Error(queryErr)
	} else {
		failures := []map[string]any{}
		for rows.Next() {
			var status, reason string
			var count int
			if rows.Scan(&status, &reason, &count) == nil {
				failures = append(failures, map[string]any{"status": status, "reason": reason, "count": count})
			}
		}
		rows.Close()
		report["audit_failures"] = failures
	}
	report["duplicate_executions"] = rest.duplicate.Load() + native.duplicate.Load()
	if rest.duplicate.Load()+native.duplicate.Load() != 0 {
		t.Error("duplicate execution")
	}
	// Retain authenticated status without retaining secrets.
	request, _ := http.NewRequestWithContext(ctx, "GET", omniURL+"/internal/status", nil)
	request.Header.Set("X-VMCP-Omni-Secret", os.Getenv("VMCP_ACCEPTANCE_OMNI_SECRET"))
	if response, err := client.Do(request); err == nil {
		b, _ := io.ReadAll(response.Body)
		response.Body.Close()
		report["index_status"] = json.RawMessage(b)
	}
}
