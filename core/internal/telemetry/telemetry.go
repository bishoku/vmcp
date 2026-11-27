// Package telemetry exports bounded, payload-free operational measurements.
package telemetry

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var bounds = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60, 90}

type Registry struct {
	operations         map[Operation]*operationSeries
	operationOverflows uint64
	operationsPruned   time.Time
	mu                 sync.Mutex
	started            time.Time
	gaugeMu            sync.RWMutex
	gauges             func() map[string]float64
	handler            http.Handler
	httpActive         metric.Int64UpDownCounter
	httpErrors         metric.Int64Counter
	httpDuration       metric.Float64Histogram
	background         metric.Int64Counter
}

func (r *Registry) SetGauges(read func() map[string]float64) {
	r.gaugeMu.Lock()
	defer r.gaugeMu.Unlock()
	r.gauges = read
}

func route(path string) string {
	switch {
	case strings.HasPrefix(path, "/mcp/"):
		return "mcp"
	case strings.HasPrefix(path, "/api/"):
		return "api"
	case path == "/healthz" || path == "/readyz":
		return "probe"
	default:
		return "other"
	}
}

type writer struct {
	http.ResponseWriter
	status int
}

func (w *writer) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *writer) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
		w.ResponseWriter.WriteHeader(code)
	}
}
func (w *writer) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(b)
}
func (r *Registry) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/metrics" {
			r.ServeHTTP(w, req)
			return
		}
		class := route(req.URL.Path)
		attrs := metric.WithAttributes(attribute.String("route", class))
		r.httpActive.Add(req.Context(), 1, attrs)
		start := time.Now()
		ctx := propagation.TraceContext{}.Extract(req.Context(), propagation.HeaderCarrier(req.Header))
		ctx, span := otel.Tracer("vmcp/http").Start(ctx, class, trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(attribute.String("vmcp.route_class", class)))
		defer span.End()
		rw := &writer{ResponseWriter: w}
		defer func() {
			elapsed := time.Since(start).Seconds()
			code := rw.status
			if code == 0 {
				code = 200
			}
			span.SetAttributes(attribute.Int("http.response.status_code", code))
			r.httpActive.Add(ctx, -1, attrs)
			var failed int64
			if code >= 400 {
				failed = 1
			}
			r.httpErrors.Add(ctx, failed, attrs)
			r.httpDuration.Record(ctx, elapsed, attrs)
			slog.DebugContext(ctx, "HTTP request completed", "route", class, "status", code)
		}()
		traced := req.WithContext(ctx)
		traced.Header = req.Header.Clone()
		propagation.TraceContext{}.Inject(ctx, propagation.HeaderCarrier(traced.Header))
		next.ServeHTTP(rw, traced)

	})
}
func (r *Registry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.handler.ServeHTTP(w, req)
}

func (w *writer) Flush() {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}
