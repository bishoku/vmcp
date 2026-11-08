package telemetry

import (
	"bytes"
	"context"
	"errors"
	"go.opentelemetry.io/otel/trace"
	"log/slog"
	"strings"
	"testing"
)

func TestLoggingLevelAndTrace(t *testing.T) {
	t.Setenv("VMCP_LOG_LEVEL", "invalid")
	if _, err := NewLogger("test"); err == nil {
		t.Fatal("invalid level accepted")
	}
	t.Setenv("VMCP_LOG_LEVEL", "DEBUG")
	log, err := NewLogger("test")
	if err != nil || !log.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal(err)
	}
	var out bytes.Buffer
	h := traceHandler{slog.NewJSONHandler(&out, nil)}
	id, _ := trace.TraceIDFromHex("12345678901234567890123456789012")
	span, _ := trace.SpanIDFromHex("1234567890123456")
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: id, SpanID: span}))
	slog.New(h).With("service", "test").InfoContext(ctx, "test")
	if !strings.Contains(out.String(), id.String()) {
		t.Fatal("missing trace", out.String())
	}
}

func TestLogErrorRedactsCredential(t *testing.T) {
	var out bytes.Buffer
	log := slog.New(traceHandler{slog.NewJSONHandler(&out, nil)}).With("err", errors.New("DSN SECRET"))
	log.Error("failed", "nested", slog.GroupValue(slog.Any("error", errors.New("PAYLOAD SECRET"))))
	if strings.Contains(out.String(), "SECRET") || !strings.Contains(out.String(), "errorString") {
		t.Fatal(out.String())
	}
}
