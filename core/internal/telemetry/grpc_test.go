package telemetry

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"go.opentelemetry.io/otel"
	logs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	traces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
)

type logsReceiver struct {
	logs.UnimplementedLogsServiceServer
	received atomic.Int64
}

func (s *logsReceiver) Export(_ context.Context, req *logs.ExportLogsServiceRequest) (*logs.ExportLogsServiceResponse, error) {
	s.received.Add(int64(len(req.GetResourceLogs())))
	return &logs.ExportLogsServiceResponse{}, nil
}

type metricsReceiver struct {
	metrics.UnimplementedMetricsServiceServer
	received atomic.Int64
}

func (s *metricsReceiver) Export(_ context.Context, req *metrics.ExportMetricsServiceRequest) (*metrics.ExportMetricsServiceResponse, error) {
	s.received.Add(int64(len(req.GetResourceMetrics())))
	return &metrics.ExportMetricsServiceResponse{}, nil
}

type tracesReceiver struct {
	traces.UnimplementedTraceServiceServer
	received atomic.Int64
}

func (s *tracesReceiver) Export(_ context.Context, req *traces.ExportTraceServiceRequest) (*traces.ExportTraceServiceResponse, error) {
	s.received.Add(int64(len(req.GetResourceSpans())))
	return &traces.ExportTraceServiceResponse{}, nil
}

func TestGRPCExportsAllSignals(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	lr := &logsReceiver{}
	mr := &metricsReceiver{}
	tr := &tracesReceiver{}
	logs.RegisterLogsServiceServer(srv, lr)
	metrics.RegisterMetricsServiceServer(srv, mr)
	traces.RegisterTraceServiceServer(srv, tr)
	go srv.Serve(listener)
	defer srv.Stop()

	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://"+listener.Addr().String())
	t.Setenv("OTEL_TRACES_SAMPLER", "always_on")
	t.Setenv("OTEL_METRIC_EXPORT_INTERVAL", "600000")
	old := otel.GetTracerProvider()
	defer otel.SetTracerProvider(old)
	sdk, err := Start(t.Context(), "grpc-test", "dev")
	if err != nil {
		t.Fatal(err)
	}
	sdk.Metrics.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sdk.Log.InfoContext(r.Context(), "grpc request")
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/test", nil))
	sdk.Close()

	// The shutdown flush completes each Export RPC before these counts are read.
	if lr.received.Load() == 0 || mr.received.Load() == 0 || tr.received.Load() == 0 {
		t.Fatalf("missing gRPC data: logs=%d metrics=%d traces=%d", lr.received.Load(), mr.received.Load(), tr.received.Load())
	}
}
