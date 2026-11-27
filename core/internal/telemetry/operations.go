package telemetry

import (
	"sort"
	"sync"
	"time"
)

// Operation labels must come from saved configuration, never request arguments,
// arbitrary tool names, resource URIs, identities, credentials or error text.
type Operation struct{ Family, VirtualServer, Upstream, Method, Tool string }
type operationSeries struct {
	start   time.Time
	active  int64
	count   map[string]int64
	sum     float64
	buckets []uint64
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
			s = &operationSeries{start: started, count: make(map[string]int64), buckets: make([]uint64, len(bounds)+1)}
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
			s.buckets[sort.SearchFloat64s(bounds, elapsed)]++
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
