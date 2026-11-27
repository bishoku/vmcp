package telemetry

import (
	"context"
	"maps"
	"slices"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// Produce imports point-in-time service stats and the expiring operation table
// into both SDK readers. Cumulative synchronous histograms retain idle labels;
// using a Producer here preserves the shared series budget and one-hour expiry.
func (r *Registry) Produce(ctx context.Context) ([]metricdata.ScopeMetrics, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	now := time.Now()
	scope := metricdata.ScopeMetrics{Scope: instrumentation.Scope{Name: "vmcp/metrics"}}
	stats := runtimeMetrics()
	r.gaugeMu.RLock()
	read := r.gauges
	r.gaugeMu.RUnlock()
	if read != nil {
		for name, v := range read() {
			stats["vmcp_"+name] = v
		}
	}
	for name, v := range stats {
		points := []metricdata.DataPoint[float64]{{StartTime: r.started, Time: now, Value: v}}
		m := metricdata.Metrics{Name: name, Data: metricdata.Gauge[float64]{DataPoints: points}}
		if strings.HasSuffix(name, "_total") {
			m.Data = metricdata.Sum[float64]{DataPoints: points, Temporality: metricdata.CumulativeTemporality, IsMonotonic: true}
		}
		scope.Metrics = append(scope.Metrics, m)
	}
	operations, overflows := r.snapshotOperations(now)
	overflow := metricdata.Sum[int64]{IsMonotonic: true, Temporality: metricdata.CumulativeTemporality, DataPoints: []metricdata.DataPoint[int64]{{Time: now, StartTime: r.started, Value: int64(overflows)}}}
	scope.Metrics = append(scope.Metrics, metricdata.Metrics{Name: "vmcp_operation_label_overflows_total", Data: overflow})
	for _, family := range []string{"virtual_requests", "virtual_tool_calls", "upstream_requests"} {
		active := metricdata.Gauge[int64]{}
		totals := metricdata.Sum[int64]{IsMonotonic: true, Temporality: metricdata.CumulativeTemporality}
		duration := metricdata.Histogram[float64]{Temporality: metricdata.CumulativeTemporality}
		for key, s := range operations {
			if key.Family != family {
				continue
			}
			attrs := key.attributes()
			set := attribute.NewSet(attrs...)
			active.DataPoints = append(active.DataPoints, metricdata.DataPoint[int64]{Attributes: set, StartTime: s.start, Time: now, Value: s.active})
			var count uint64
			for outcome, n := range s.count {
				count += uint64(n)
				totals.DataPoints = append(totals.DataPoints, metricdata.DataPoint[int64]{Attributes: attribute.NewSet(append(attrs, attribute.String("outcome", outcome))...), StartTime: s.start, Time: now, Value: n})
			}
			duration.DataPoints = append(duration.DataPoints, metricdata.HistogramDataPoint[float64]{Attributes: set, StartTime: s.start, Time: now, Count: count, Sum: s.sum, Bounds: bounds, BucketCounts: s.buckets})
		}
		if len(active.DataPoints) > 0 {
			prefix := "vmcp_" + family
			scope.Metrics = append(scope.Metrics, metricdata.Metrics{Name: prefix + "_active", Data: active}, metricdata.Metrics{Name: prefix + "_total", Data: totals}, metricdata.Metrics{Name: prefix + "_duration_seconds", Unit: "s", Data: duration})
		}
	}
	return []metricdata.ScopeMetrics{scope}, nil
}
func (o Operation) attributes() []attribute.KeyValue {
	switch o.Family {
	case "virtual_requests":
		return []attribute.KeyValue{attribute.String("virtual_server", o.VirtualServer)}
	case "virtual_tool_calls":
		return []attribute.KeyValue{attribute.String("virtual_server", o.VirtualServer), attribute.String("tool", o.Tool)}
	default:
		return []attribute.KeyValue{attribute.String("upstream", o.Upstream), attribute.String("method", o.Method), attribute.String("tool", o.Tool)}
	}
}

func (r *Registry) snapshotOperations(now time.Time) (map[Operation]operationSeries, uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneOperations(now)
	snapshot := make(map[Operation]operationSeries, len(r.operations))
	for key, value := range r.operations {
		v := *value
		v.count = maps.Clone(value.count)
		v.buckets = slices.Clone(value.buckets)
		snapshot[key] = v
	}
	return snapshot, r.operationOverflows
}
