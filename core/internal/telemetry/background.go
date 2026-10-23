package telemetry

import (
	"fmt"
	"maps"
	"net/http"
)

// Background counts maintenance outcomes with a fixed, payload-free label set.
// It deliberately has no session, resource URI, identity or error-text labels.
func (r *Registry) Background(activity, outcome string) {
	if r == nil {
		return
	}
	if activity != "discovery" && activity != "subscription" {
		return
	}
	switch outcome {
	case "changed", "unchanged", "ok", "error", "cancelled":
	default:
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.background == nil {
		r.background = map[string]uint64{}
	}
	r.background[activity+":"+outcome]++
}
func (r *Registry) backgroundMetrics(w http.ResponseWriter) {
	r.mu.Lock()
	counts := maps.Clone(r.background)
	r.mu.Unlock()
	fmt.Fprintln(w, "# HELP vmcp_background_operations_total Background maintenance outcomes (no audit lifecycle).")
	fmt.Fprintln(w, "# TYPE vmcp_background_operations_total counter")
	for _, activity := range []string{"discovery", "subscription"} {
		for _, outcome := range []string{"changed", "unchanged", "ok", "error", "cancelled"} {
			if count, ok := counts[activity+":"+outcome]; ok {
				fmt.Fprintf(w, "vmcp_background_operations_total{activity=%q,outcome=%q} %d\n", activity, outcome, count)
			}
		}
	}
}
