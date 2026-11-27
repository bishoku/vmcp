package telemetry

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	logs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	traces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestDirectOTLP(t *testing.T) {
	for _, protocol := range []string{"", "http/protobuf"} {
		name := protocol
		if name == "" {
			name = "default"
		}
		t.Run(name, func(t *testing.T) { t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", protocol); testHTTPExport(t) })
	}
}

func testHTTPExport(t *testing.T) {
	old := otel.GetTracerProvider()
	defer otel.SetTracerProvider(old)
	var lock sync.Mutex
	bodies := map[string][]byte{}
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test") != "ok" {
			t.Error("missing configured exporter header")
		}
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		lock.Lock()
		bodies[r.URL.Path] = b
		lock.Unlock()
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer collector.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "x-test=ok")
	t.Setenv("OTEL_TRACES_SAMPLER", "always_on")
	t.Setenv("OTEL_SERVICE_NAME", "configured-service")
	t.Setenv("OTEL_METRIC_EXPORT_INTERVAL", "600000")
	sdk, err := Start(t.Context(), "test", "dev")
	if err != nil {
		t.Fatal(err)
	}
	sdk.Metrics.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sdk.Log.InfoContext(r.Context(), "completed", "error", errors.New("secret-payload"))
		sdk.Log.DebugContext(r.Context(), "filtered-debug")
		sdk.Metrics.BeginOperation(Operation{Family: "virtual_requests", VirtualServer: "demo"})("ok")
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/example", nil))
	_ = scrapeOperations(sdk.Metrics)
	sdk.Close()
	lock.Lock()
	defer lock.Unlock()
	var tr traces.ExportTraceServiceRequest
	var lr logs.ExportLogsServiceRequest
	var mr metrics.ExportMetricsServiceRequest
	for path, msg := range map[string]proto.Message{"/v1/traces": &tr, "/v1/logs": &lr, "/v1/metrics": &mr} {
		b := bodies[path]
		if len(b) == 0 {
			t.Fatalf("missing %s", path)
		}
		if err := proto.Unmarshal(b, msg); err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(b, []byte("secret-payload")) || bytes.Contains(b, []byte("filtered-debug")) {
			t.Fatalf("unsanitized or unfiltered %s", path)
		}
		if !bytes.Contains(b, []byte("configured-service")) {
			t.Fatalf("missing service identity in %s", path)
		}
	}
	span := tr.ResourceSpans[0].ScopeSpans[0].Spans[0]
	record := lr.ResourceLogs[0].ScopeLogs[0].LogRecords[0]
	if !bytes.Equal(span.TraceId, record.TraceId) || !bytes.Equal(span.SpanId, record.SpanId) || len(record.TraceId) != 16 {
		t.Fatal("log is not correlated to request span")
	}
	counts := map[string]uint64{}
	for _, res := range mr.ResourceMetrics {
		for _, scope := range res.ScopeMetrics {
			for _, m := range scope.Metrics {
				for _, p := range m.GetHistogram().GetDataPoints() {
					counts[m.Name] += p.Count
				}
			}
		}
	}
	for _, name := range []string{"vmcp_http_duration_seconds", "vmcp_virtual_requests_duration_seconds"} {
		if counts[name] != 1 {
			t.Fatalf("%s count = %d", name, counts[name])
		}
	}
}

func TestSignalSelection(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "https://example.invalid/v1/traces")
	t.Setenv("VMCP_OTEL_ENABLED", "true")
	selected, err := signals()
	if err != nil || len(selected) != 1 || selected["TRACES"] != httpProtobuf {
		t.Fatal(selected, err)
	}
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "https://example.invalid")
	t.Setenv("OTEL_LOGS_EXPORTER", "none")
	selected, err = signals()
	if err != nil || len(selected) != 2 || selected["METRICS"] != httpProtobuf {
		t.Fatal(selected, err)
	}
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "http/protobuf")
	selected, err = signals()
	if err != nil || len(selected) != 2 || selected["METRICS"] != grpcTransport || selected["TRACES"] != httpProtobuf {
		t.Fatal(selected, err)
	}
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_PROTOCOL", "unsupported")
	if _, err = signals(); err == nil {
		t.Fatal("accepted unsupported metrics protocol")
	}
	t.Setenv("VMCP_OTEL_ENABLED", "false")
	selected, err = signals()
	if err != nil || len(selected) != 0 {
		t.Fatal(selected, err)
	}
}

func TestCollectorOutageKeepsServing(t *testing.T) {
	old := otel.GetTracerProvider()
	defer otel.SetTracerProvider(old)
	started := make(chan struct{})
	var once sync.Once
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(started) })
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer collector.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)
	t.Setenv("OTEL_METRIC_EXPORT_INTERVAL", "10")
	sdk, err := Start(t.Context(), "test", "dev")
	if err != nil {
		t.Fatal(err)
	}
	defer sdk.Close()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("reader did not export")
	}
	w := httptest.NewRecorder()
	sdk.Metrics.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })).ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != 200 {
		t.Fatalf("during collector outage: status %d", w.Code)
	}
}
