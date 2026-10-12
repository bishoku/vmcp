// Package telemetry exports bounded, payload-free operational measurements.
package telemetry

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

var bounds = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60, 90}

type series struct {
	count, active int64
	sum           float64
	buckets       []int64
	errors        int64
}
type Registry struct {
	operations         map[Operation]*operationSeries
	operationOverflows uint64
	operationsPruned   time.Time
	mu                 sync.Mutex
	requests           map[string]*series
	Gauges             func(context.Context) map[string]float64
}

func New() *Registry { return &Registry{requests: map[string]*series{}} }
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
		r.mu.Lock()
		s := r.requests[class]
		if s == nil {
			s = &series{buckets: make([]int64, len(bounds))}
			r.requests[class] = s
		}
		s.active++
		r.mu.Unlock()
		start := time.Now()
		ctx := propagation.TraceContext{}.Extract(req.Context(), propagation.HeaderCarrier(req.Header))
		ctx, span := otel.Tracer("vmcp/http").Start(ctx, class, trace.WithAttributes(attribute.String("vmcp.route_class", class)))
		defer span.End()
		rw := &writer{ResponseWriter: w}
		defer func() {
			elapsed := time.Since(start).Seconds()
			code := rw.status
			if code == 0 {
				code = 200
			}
			span.SetAttributes(attribute.Int("http.response.status_code", code))
			r.mu.Lock()
			defer r.mu.Unlock()
			s.active--
			s.count++
			s.sum += elapsed
			if code >= 400 {
				s.errors++
			}
			for i, b := range bounds {
				if elapsed <= b {
					s.buckets[i]++
				}
			}
		}()
		traced := req.WithContext(ctx)
		traced.Header = req.Header.Clone()
		propagation.TraceContext{}.Inject(ctx, propagation.HeaderCarrier(traced.Header))
		next.ServeHTTP(rw, traced)

	})
}
func (r *Registry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	r.mu.Lock()
	fmt.Fprintln(w, "# TYPE vmcp_http_duration_seconds histogram")
	for label, s := range r.requests {
		fmt.Fprintf(w, "vmcp_http_active{route=%q} %d\nvmcp_http_errors_total{route=%q} %d\n", label, s.active, label, s.errors)
		for i, b := range bounds {
			fmt.Fprintf(w, "vmcp_http_duration_seconds_bucket{route=%q,le=%q} %d\n", label, strconv.FormatFloat(b, 'g', -1, 64), s.buckets[i])
		}
		fmt.Fprintf(w, "vmcp_http_duration_seconds_bucket{route=%q,le=\"+Inf\"} %d\nvmcp_http_duration_seconds_count{route=%q} %d\nvmcp_http_duration_seconds_sum{route=%q} %g\n", label, s.count, label, s.count, label, s.sum)
	}
	r.mu.Unlock()
	r.serveOperations(w)
	if r.Gauges != nil {
		ctx, cancel := context.WithTimeout(req.Context(), time.Second)
		defer cancel()
		for name, value := range r.Gauges(ctx) {
			fmt.Fprintf(w, "vmcp_%s %g\n", name, value)
		}
	}
}

// InitTracing is opt-in. SDK queues are bounded; collector outage does not block requests.
func InitTracing(ctx context.Context, service string) (func(context.Context) error, error) {
	enabled := os.Getenv("VMCP_OTEL_ENABLED")
	if enabled == "" || enabled == "false" {
		return func(context.Context) error { return nil }, nil
	}
	if enabled != "true" {
		return nil, fmt.Errorf("VMCP_OTEL_ENABLED must be true or false")
	}
	if os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") == "" {
		return nil, fmt.Errorf("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT is required")
	}
	ratio := .1
	if raw := os.Getenv("VMCP_OTEL_SAMPLE_RATIO"); raw != "" {
		var err error
		ratio, err = strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 || ratio > 1 {
			return nil, fmt.Errorf("VMCP_OTEL_SAMPLE_RATIO must be between 0 and 1")
		}
	}
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithTimeout(2*time.Second))
	if err != nil {
		return nil, err
	}
	provider := sdktrace.NewTracerProvider(sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", service))), sdktrace.WithSampler(sdktrace.TraceIDRatioBased(ratio)), sdktrace.WithBatcher(exporter, sdktrace.WithMaxQueueSize(2048), sdktrace.WithMaxExportBatchSize(256), sdktrace.WithExportTimeout(2*time.Second)))
	otel.SetTracerProvider(provider)
	return provider.Shutdown, nil
}

func (w *writer) Flush() {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}
