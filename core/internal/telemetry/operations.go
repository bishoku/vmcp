package telemetry

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Operation labels must come from saved configuration, never request arguments,
// arbitrary tool names, resource URIs, identities, credentials or error text.
type Operation struct{ Family, VirtualServer, Upstream, Method, Tool string }
type operationSeries struct {
	active  int64
	count   map[string]int64
	sum     float64
	buckets []int64
	last    time.Time
}

const maxOperationSeries = 2048
const operationIdleTTL = time.Hour

// BeginOperation counts logical gateway work. Upstream observations include
// connection setup and session recovery; outer resilience retries count again.
// Idle series expire after an hour; excess new label sets share overflow series.
func (r *Registry) BeginOperation(labels Operation) func(string) {
	if r == nil {
		return func(string) {}
	}
	switch labels.Family {
	case "virtual_requests", "virtual_tool_calls", "upstream_requests":
	default:
		return func(string) {}
	}
	// Discard unused dimensions and bound configuration-name lengths too.
	switch labels.Family {
	case "virtual_requests":
		labels.Upstream, labels.Method, labels.Tool = "", "", ""
	case "virtual_tool_calls":
		labels.Upstream, labels.Method = "", ""
	case "upstream_requests":
		labels.VirtualServer = ""
	}
	oversized := len(labels.VirtualServer) > 256 || len(labels.Upstream) > 256 || len(labels.Method) > 256 || len(labels.Tool) > 256
	if oversized {
		labels = Operation{Family: labels.Family, VirtualServer: "__overflow__", Upstream: "__overflow__", Method: "__overflow__", Tool: "__overflow__"}
	}
	started := time.Now()
	r.mu.Lock()
	r.pruneOperations(started)
	if oversized {
		r.operationOverflows++
	}
	if r.operations == nil {
		r.operations = make(map[Operation]*operationSeries)
	}
	s := r.operations[labels]
	if s == nil {
		if len(r.operations) >= maxOperationSeries {
			r.operationOverflows++
			labels = Operation{Family: labels.Family, VirtualServer: "__overflow__", Upstream: "__overflow__", Method: "__overflow__", Tool: "__overflow__"}
			s = r.operations[labels]
		}
		if s == nil {
			s = &operationSeries{count: make(map[string]int64), buckets: make([]int64, len(bounds))}
			r.operations[labels] = s
		}
	}
	s.active++
	s.last = started
	r.mu.Unlock()
	var once sync.Once
	return func(outcome string) {
		once.Do(func() {
			switch outcome {
			case "ok", "tool_error", "denied", "cancelled":
			default:
				outcome = "error"
			}
			elapsed := time.Since(started).Seconds()
			r.mu.Lock()
			defer r.mu.Unlock()
			s.active--
			s.count[outcome]++
			s.sum += elapsed
			s.last = time.Now()
			for i, b := range bounds {
				if elapsed <= b {
					s.buckets[i]++
				}
			}
		})
	}
}
func (r *Registry) pruneOperations(now time.Time) {
	if now.Sub(r.operationsPruned) < time.Minute {
		return
	}
	for key, s := range r.operations {
		if s.active == 0 && now.Sub(s.last) >= operationIdleTTL {
			delete(r.operations, key)
		}
	}
	r.operationsPruned = now
}
func (r *Registry) serveOperations(w http.ResponseWriter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneOperations(time.Now())
	fmt.Fprintln(w, "# HELP vmcp_operation_label_overflows_total Total number of operations routed to the overflow label bucket.")
	fmt.Fprintln(w, "# TYPE vmcp_operation_label_overflows_total counter")
	fmt.Fprintf(w, "vmcp_operation_label_overflows_total %d\n", r.operationOverflows)
	for _, family := range []string{"virtual_requests", "virtual_tool_calls", "upstream_requests"} {
		prefix := "vmcp_" + family
		fmt.Fprintf(w, "# HELP %s_total Total operations for %s.\n# TYPE %s_total counter\n# HELP %s_active Active operations for %s.\n# TYPE %s_active gauge\n# HELP %s_duration_seconds Operation duration seconds for %s.\n# TYPE %s_duration_seconds histogram\n", prefix, family, prefix, prefix, family, prefix, prefix, family, prefix)
		var keys []Operation
		for key := range r.operations {
			if key.Family == family {
				keys = append(keys, key)
			}
		}
		sort.Slice(keys, func(i, j int) bool {
			a, b := keys[i], keys[j]
			if a.VirtualServer != b.VirtualServer {
				return a.VirtualServer < b.VirtualServer
			}
			if a.Upstream != b.Upstream {
				return a.Upstream < b.Upstream
			}
			if a.Method != b.Method {
				return a.Method < b.Method
			}
			return a.Tool < b.Tool
		})
		for _, key := range keys {
			s := r.operations[key]
			labels := ""
			switch family {
			case "virtual_requests":
				labels = fmt.Sprintf("virtual_server=%s", metricLabel(key.VirtualServer))
			case "virtual_tool_calls":
				labels = fmt.Sprintf("virtual_server=%s,tool=%s", metricLabel(key.VirtualServer), metricLabel(key.Tool))
			case "upstream_requests":
				labels = fmt.Sprintf("upstream=%s,method=%s,tool=%s", metricLabel(key.Upstream), metricLabel(key.Method), metricLabel(key.Tool))
			}
			fmt.Fprintf(w, "%s_active{%s} %d\n", prefix, labels, s.active)
			outcomes := make([]string, 0, len(s.count))
			for outcome := range s.count {
				outcomes = append(outcomes, outcome)
			}
			sort.Strings(outcomes)
			var count int64
			for _, outcome := range outcomes {
				n := s.count[outcome]
				count += n
				fmt.Fprintf(w, "%s_total{%s,outcome=%q} %d\n", prefix, labels, outcome, n)
			}
			for i, b := range bounds {
				fmt.Fprintf(w, "%s_duration_seconds_bucket{%s,le=%q} %d\n", prefix, labels, strconv.FormatFloat(b, 'g', -1, 64), s.buckets[i])
			}
			fmt.Fprintf(w, "%s_duration_seconds_bucket{%s,le=\"+Inf\"} %d\n%s_duration_seconds_count{%s} %d\n%s_duration_seconds_sum{%s} %g\n", prefix, labels, count, prefix, labels, count, prefix, labels, s.sum)
		}
	}
}

func metricLabel(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`).Replace(value) + `"`
}
