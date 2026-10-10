package store

import "time"

// NodeHealth is what one data-plane node last saw when probing an upstream.
type NodeHealth struct {
	Node      string    `json:"node"`
	State     string    `json:"state"` // healthy | unhealthy
	Breaker   string    `json:"breaker"`
	URL       string    `json:"url"`
	LatencyMS int       `json:"latencyMs"`
	Error     *string   `json:"error"`
	CheckedAt time.Time `json:"checkedAt"`
}

// Health summarizes the recent probes of all nodes.
type Health struct {
	// State is unknown (no recent probe), healthy, degraded (nodes disagree) or unhealthy.
	State string       `json:"state"`
	Nodes []NodeHealth `json:"nodes"`
}

func (h *Health) summarize() {
	if h.Nodes == nil {
		h.Nodes = []NodeHealth{}
	}
	healthy := 0
	for _, n := range h.Nodes {
		if n.State == "healthy" {
			healthy++
		}
	}
	switch {
	case len(h.Nodes) == 0:
		h.State = "unknown"
	case healthy == len(h.Nodes):
		h.State = "healthy"
	case healthy == 0:
		h.State = "unhealthy"
	default:
		h.State = "degraded"
	}
}
