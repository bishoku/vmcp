package telemetry

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func scrapeOperations(r *Registry) string {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	return w.Body.String()
}
func TestOperationMetricsConcurrentOutcomes(t *testing.T) {
	r := New()
	labels := Operation{Family: "virtual_tool_calls", VirtualServer: "support", Tool: "lookup"}
	var workers sync.WaitGroup
	for i := 0; i < 50; i++ {
		workers.Go(func() { done := r.BeginOperation(labels); done("ok"); done("error") })
	}
	workers.Wait()
	done := r.BeginOperation(labels)
	body := scrapeOperations(r)
	for _, want := range []string{`vmcp_virtual_tool_calls_active{virtual_server="support",tool="lookup"} 1`, `vmcp_virtual_tool_calls_total{virtual_server="support",tool="lookup",outcome="ok"} 50`, `vmcp_virtual_tool_calls_duration_seconds_bucket{virtual_server="support",tool="lookup",le="+Inf"} 50`} {
		if !strings.Contains(body, want) {
			t.Fatal(want, body)
		}
	}
	done("raw-secret-error")
	body = scrapeOperations(r)
	if strings.Contains(body, "raw-secret") || !strings.Contains(body, `outcome="error"} 1`) {
		t.Fatal(body)
	}
}
func TestOperationMetricsBoundAndExpireIdleSeries(t *testing.T) {
	r := New()
	for i := 0; i < maxOperationSeries+20; i++ {
		r.BeginOperation(Operation{Family: "upstream_requests", Upstream: fmt.Sprint(i), Method: "tools/call", Tool: "read"})("ok")
	}
	if len(r.operations) > maxOperationSeries+3 || r.operationOverflows != 20 {
		t.Fatalf("unbounded labels: %d %d", len(r.operations), r.operationOverflows)
	}
	active := Operation{Family: "virtual_requests", VirtualServer: "active"}
	// Age all existing entries, then retain one active series through cleanup.
	for _, s := range r.operations {
		s.last = time.Now().Add(-2 * time.Hour)
	}
	r.operationsPruned = time.Time{}
	done := r.BeginOperation(active)
	for _, s := range r.operations {
		s.last = time.Now().Add(-2 * time.Hour)
	}
	r.operationsPruned = time.Time{}
	_ = scrapeOperations(r)
	if len(r.operations) != 1 {
		t.Fatal("idle cleanup removed active work or retained expired labels")
	}
	done("ok")
	var nilRegistry *Registry
	nilRegistry.BeginOperation(active)("ok")
}
func TestOperationMetricsEscapeConfiguredNames(t *testing.T) {
	r := New()
	r.BeginOperation(Operation{Family: "virtual_tool_calls", VirtualServer: "demo", Tool: "quoted\"\\\nname"})("ok")
	body := scrapeOperations(r)
	if !strings.Contains(body, `tool="quoted\"\\\nname"`) {
		t.Fatal(body)
	}
}
func TestOperationMetricNamesAreLengthBounded(t *testing.T) {
	r := New()
	r.BeginOperation(Operation{Family: "virtual_tool_calls", VirtualServer: "demo", Tool: strings.Repeat("x", 4096)})("ok")
	if len(r.operations) != 1 || r.operationOverflows != 1 || strings.Contains(scrapeOperations(r), strings.Repeat("x", 257)) {
		t.Fatal("oversized configuration label retained")
	}
}
