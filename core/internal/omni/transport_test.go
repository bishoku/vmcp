package omni

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/bishoku/vmcp/core/internal/telemetry"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestProviderRequiresEgressGrantAndAcceptsPrivateCA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"index":0,"embedding":[1]}]}`))
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VMCP_OMNI_EMBEDDING_CA_FILE", path)
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "")
	e, err := NewHTTPEmbedder(srv.URL, "model", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Embed(context.Background(), []string{"query"}); err == nil {
		t.Fatal("private destination allowed without grant")
	}
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	e, err = NewHTTPEmbedder(srv.URL, "model", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Embed(context.Background(), []string{"query"}); err != nil {
		t.Fatal("trusted private CA failed", err)
	}
	if _, err = NewHTTPEmbedder(srv.URL, "model", "", true); err == nil {
		t.Fatal("CA with verification bypass")
	}
}

func TestGatewayTracePropagation(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	old := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer func() { otel.SetTracerProvider(old); _ = provider.Shutdown(context.Background()) }()
	registry := telemetry.New()
	dataPlane := httptest.NewServer(registry.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })))
	defer dataPlane.Close()
	b := Backend{URL: dataPlane.URL, Client: dataPlane.Client()}
	ctx, root := provider.Tracer("test").Start(t.Context(), "omni")
	if err := b.Request(ctx, "POST", "/internal/omni/invoke", "", "", "", nil, nil); err != nil {
		t.Fatal(err)
	}
	root.End()
	ended := recorder.Ended()
	if len(ended) != 3 {
		t.Fatalf("expected server, client and root spans, got %d", len(ended))
	}
	server, client := ended[0], ended[1]
	if server.Parent().SpanID() != client.SpanContext().SpanID() || client.Parent().SpanID() != root.SpanContext().SpanID() || server.SpanContext().TraceID() != root.SpanContext().TraceID() {
		t.Fatal("broken internal trace chain")
	}
}
