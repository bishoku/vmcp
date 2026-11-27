package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/otlptranslator"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type SDK struct {
	Metrics  *Registry
	Log      *slog.Logger
	shutdown []func(context.Context) error
}

// Run owns SDK setup, fatal logging and the final flush before main exits.
func Run(service, version string, serve func(*SDK) error) int {
	sdk, err := Start(context.Background(), service, version)
	if err != nil {
		slog.Error("telemetry initialization failed", "error_type", fmt.Sprintf("%T", err))
		return 1
	}
	defer sdk.Close()
	slog.SetDefault(sdk.Log)
	if err := serve(sdk); err != nil {
		sdk.Log.Error("service stopped", "error", err)
		return 1
	}
	return 0
}

// Start configures standard OTLP/HTTP exporters only for explicitly configured
// destinations. The private Prometheus reader is always available.
func Start(ctx context.Context, service, version string) (sdk *SDK, err error) {
	enabled, err := signals()
	if err != nil {
		return nil, err
	}
	attrs := []attribute.KeyValue{attribute.String("service.name", "vmcp-"+service)}
	if version != "" {
		attrs = append(attrs, attribute.String("service.version", version))
	}
	res, err := resource.New(ctx, resource.WithTelemetrySDK(), resource.WithAttributes(attrs...), resource.WithFromEnv())
	if err != nil {
		return nil, err
	}
	s := &SDK{}
	defer func() {
		if err != nil {
			s.Close()
		}
	}()
	// Keep SDK diagnostics out of its own log exporter. Remote error messages
	// and endpoint URLs can contain private data.
	if len(enabled) > 0 {
		local := slog.New(slog.NewJSONHandler(os.Stderr, nil))
		otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) { local.Warn("OpenTelemetry SDK error", "error_type", fmt.Sprintf("%T", err)) }))
	}
	var extra slog.Handler
	if enabled["LOGS"] {
		exp, e := otlploghttp.New(ctx)
		if e != nil {
			return nil, e
		}
		lp := sdklog.NewLoggerProvider(sdklog.WithResource(res), sdklog.WithProcessor(sdklog.NewBatchProcessor(exp)))
		s.shutdown = append(s.shutdown, lp.Shutdown)
		extra = otelslog.NewHandler("vmcp/slog", otelslog.WithLoggerProvider(lp))
	}
	s.Log, err = newLogger(service, extra)
	if err != nil {
		return nil, err
	}
	r := &Registry{started: time.Now()}
	var opts []sdkmetric.Option
	if enabled["METRICS"] {
		exp, e := otlpmetrichttp.New(ctx)
		if e != nil {
			return nil, e
		}
		opts = append(opts, sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp, sdkmetric.WithProducer(r))))
	}
	mp, err := r.initMetrics(res, opts...)
	if err != nil {
		return nil, err
	}
	s.Metrics = r
	s.shutdown = append(s.shutdown, mp.Shutdown)
	if enabled["TRACES"] {
		tp, e := startTracing(ctx, res)
		if e != nil {
			return nil, e
		}
		s.shutdown = append(s.shutdown, tp.Shutdown)
		otel.SetTracerProvider(tp)
	}
	return s, nil
}

func signals() (map[string]bool, error) {
	enabled := map[string]bool{}
	legacy := os.Getenv("VMCP_OTEL_ENABLED")
	switch legacy {
	case "false":
		return enabled, nil
	case "", "true":
	default:
		return nil, errors.New("VMCP_OTEL_ENABLED must be true or false")
	}
	if strings.EqualFold(os.Getenv("OTEL_SDK_DISABLED"), "true") {
		return enabled, nil
	}
	common := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	for _, signal := range []string{"TRACES", "LOGS", "METRICS"} {
		key := "OTEL_EXPORTER_OTLP_" + signal
		if os.Getenv(key+"_ENDPOINT") == "" && common == "" {
			continue
		}
		switch os.Getenv("OTEL_" + signal + "_EXPORTER") {
		case "none":
			continue
		case "", "otlp":
		default:
			return nil, fmt.Errorf("OTEL_%s_EXPORTER must be otlp or none", signal)
		}
		protocol := os.Getenv(key + "_PROTOCOL")
		if protocol == "" {
			protocol = os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")
		}
		if protocol != "" && protocol != "http/protobuf" {
			return nil, fmt.Errorf("%s requires OTLP http/protobuf", signal)
		}
		enabled[signal] = true
	}
	if legacy == "true" && common == "" && os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") == "" {
		return nil, errors.New("enabled legacy tracing requires an OTLP trace endpoint")
	}
	return enabled, nil
}

func startTracing(ctx context.Context, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	opts := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	if os.Getenv("OTEL_TRACES_SAMPLER") == "" {
		ratio := 0.1
		if raw := os.Getenv("VMCP_OTEL_SAMPLE_RATIO"); raw != "" {
			n, err := strconv.ParseFloat(raw, 64)
			if err != nil || math.IsNaN(n) || n < 0 || n > 1 {
				return nil, errors.New("VMCP_OTEL_SAMPLE_RATIO must be between 0 and 1")
			}
			ratio = n
		}
		opts = append(opts, sdktrace.WithSampler(sdktrace.TraceIDRatioBased(ratio)))
	}
	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	var batch []sdktrace.BatchSpanProcessorOption
	if os.Getenv("OTEL_BSP_MAX_EXPORT_BATCH_SIZE") == "" {
		batch = append(batch, sdktrace.WithMaxExportBatchSize(256))
	}
	if os.Getenv("OTEL_BSP_EXPORT_TIMEOUT") == "" {
		batch = append(batch, sdktrace.WithExportTimeout(2*time.Second))
	}
	opts = append(opts, sdktrace.WithBatcher(exp, batch...))
	return sdktrace.NewTracerProvider(opts...), nil
}

func (s *SDK) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for i := len(s.shutdown) - 1; i >= 0; i-- {
		if err := s.shutdown[i](ctx); err != nil {
			slog.Warn("telemetry shutdown incomplete")
		}
	}
}

// New creates an isolated scrape-only registry, without installing global providers.
func New() *Registry {
	r := &Registry{started: time.Now()}
	if _, err := r.initMetrics(resource.Empty()); err != nil {
		panic(err)
	}
	return r
}

func (r *Registry) initMetrics(res *resource.Resource, opts ...sdkmetric.Option) (*sdkmetric.MeterProvider, error) {
	reg := prom.NewRegistry()
	reader, err := otelprom.New(otelprom.WithRegisterer(reg), otelprom.WithoutTargetInfo(), otelprom.WithoutScopeInfo(), otelprom.WithTranslationStrategy(otlptranslator.NoTranslation), otelprom.WithProducer(r))
	if err != nil {
		return nil, err
	}
	opts = append(opts, sdkmetric.WithResource(res), sdkmetric.WithReader(reader))
	mp := sdkmetric.NewMeterProvider(opts...)
	m := mp.Meter("vmcp/metrics")
	r.httpActive, err = m.Int64UpDownCounter("vmcp_http_active")
	if err != nil {
		return nil, err
	}
	r.httpErrors, err = m.Int64Counter("vmcp_http_errors_total")
	if err != nil {
		return nil, err
	}
	r.httpDuration, err = m.Float64Histogram("vmcp_http_duration_seconds", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(bounds...))
	if err != nil {
		return nil, err
	}
	r.background, err = m.Int64Counter("vmcp_background_operations_total")
	if err != nil {
		return nil, err
	}
	r.handler = promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	return mp, nil
}
