package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Background counts maintenance outcomes with a fixed, payload-free label set.
// It deliberately has no session, resource URI, identity or error-text labels.
func (r *Registry) Background(activity, outcome string) {
	if r == nil {
		return
	}
	if activity != "discovery" && activity != "subscription" && activity != "delivery_renew" {
		return
	}
	switch outcome {
	case "changed", "unchanged", "ok", "error", "cancelled":
	default:
		return
	}
	r.background.Add(context.Background(), 1, metric.WithAttributes(attribute.String("activity", activity), attribute.String("outcome", outcome)))
}
