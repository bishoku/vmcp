package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

type traceHandler struct{ slog.Handler }

type levelHandler struct {
	slog.Handler
	min slog.Level
}

func (h levelHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return l >= h.min && h.Handler.Enabled(ctx, l)
}
func (h levelHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return levelHandler{h.Handler.WithAttrs(a), h.min}
}
func (h levelHandler) WithGroup(n string) slog.Handler {
	return levelHandler{h.Handler.WithGroup(n), h.min}
}

func (h traceHandler) Handle(ctx context.Context, record slog.Record) error {
	safe := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool { safe.AddAttrs(safeLogAttr(attr)); return true })
	record = safe
	if span := trace.SpanContextFromContext(ctx); span.IsValid() {
		record.AddAttrs(slog.String("trace_id", span.TraceID().String()), slog.String("span_id", span.SpanID().String()))
	}
	return h.Handler.Handle(ctx, record)
}
func (h traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	safe := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		safe[i] = safeLogAttr(a)
	}
	return traceHandler{h.Handler.WithAttrs(safe)}
}
func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{h.Handler.WithGroup(name)}
}
func NewLogger(service string) (*slog.Logger, error) {
	return newLogger(service, nil)
}
func newLogger(service string, extra slog.Handler) (*slog.Logger, error) {
	var level slog.Level
	value := strings.ToUpper(os.Getenv("VMCP_LOG_LEVEL"))
	if value == "" {
		value = "INFO"
	}
	if level.UnmarshalText([]byte(value)) != nil {
		return nil, errors.New("VMCP_LOG_LEVEL must be DEBUG, INFO, WARN or ERROR")
	}
	switch value {
	case "DEBUG", "INFO", "WARN", "ERROR":
	default:
		return nil, errors.New("VMCP_LOG_LEVEL must be DEBUG, INFO, WARN or ERROR")
	}
	var h slog.Handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	if extra != nil {
		h = slog.NewMultiHandler(h, levelHandler{extra, level})
	}
	return slog.New(traceHandler{h}).With("service", service), nil
}

// Unknown errors can embed DSNs, URLs or payloads. Preserve their type; callers
// should log explicit bounded diagnostic fields instead of raw error text.
func safeLogAttr(attr slog.Attr) slog.Attr {
	attr.Value = attr.Value.Resolve()
	if attr.Value.Kind() == slog.KindAny {
		if err, ok := attr.Value.Any().(error); ok {
			attr.Value = slog.StringValue(fmt.Sprintf("%T", err))
		}
	}
	if attr.Value.Kind() == slog.KindGroup {
		items := attr.Value.Group()
		safe := make([]slog.Attr, len(items))
		for i, a := range items {
			safe[i] = safeLogAttr(a)
		}
		attr.Value = slog.GroupValue(safe...)
	}
	return attr
}
