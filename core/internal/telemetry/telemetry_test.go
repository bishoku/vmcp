package telemetry

import (
	"context"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsCardinalityAndPrivacy(t *testing.T) {
	registry := New()
	h := registry.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }))
	for _, path := range []string{"/mcp/secret-tool?token=secret", "/api/secret-subject", "/unknown-secret"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", path, nil))
	}
	out := httptest.NewRecorder()
	registry.ServeHTTP(out, httptest.NewRequest("GET", "/metrics", nil))
	body := out.Body.String()
	if strings.Contains(body, "secret") || len(registry.requests) != 3 {
		t.Fatalf("unbounded labels: %s", body)
	}
	if !strings.Contains(body, `vmcp_http_errors_total{route="mcp"} 1`) {
		t.Fatal(body)
	}
	if !strings.Contains(body, `le="+Inf"`) {
		t.Fatal("histogram missing infinity bucket")
	}
}
func TestTracingConfiguration(t *testing.T) {
	t.Setenv("VMCP_OTEL_ENABLED", "false")
	stop, err := InitTracing(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	stop(context.Background())
	t.Setenv("VMCP_OTEL_ENABLED", "true")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	if _, err := InitTracing(context.Background(), "test"); err == nil {
		t.Fatal("enabled tracing without endpoint")
	}
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://127.0.0.1:4318/v1/traces")
	t.Setenv("VMCP_OTEL_SAMPLE_RATIO", "2")
	if _, err := InitTracing(context.Background(), "test"); err == nil {
		t.Fatal("invalid sampling accepted")
	}
}

func TestTraceSpansExcludeCallerSecrets(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	old := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer func() { otel.SetTracerProvider(old); _ = provider.Shutdown(context.Background()) }()
	req := httptest.NewRequest("POST", "/mcp/secret-tool?token=secret-value", strings.NewReader("secret-body"))
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("baggage", "secret=secret-baggage")
	New().Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })).ServeHTTP(httptest.NewRecorder(), req)
	spans := recorder.Ended()
	if len(spans) != 1 || spans[0].Name() != "mcp" {
		t.Fatal("missing bounded HTTP span")
	}
	for _, attr := range spans[0].Attributes() {
		if strings.Contains(attr.Value.Emit(), "secret") {
			t.Fatalf("trace leaked caller secret: %v", attr)
		}
	}
}
