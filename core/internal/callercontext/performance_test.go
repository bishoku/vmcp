package callercontext_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"testing"

	"github.com/bishoku/vmcp/core/internal/callercontext"
	"github.com/bishoku/vmcp/core/internal/upstream"
)

type forwardingFixture struct {
	config   *callercontext.Config
	meta     json.RawMessage
	rules    []callercontext.HeaderMapping
	incoming http.Header
}

func fixture(kind, scenario string) forwardingFixture {
	f := forwardingFixture{incoming: http.Header{}}
	if scenario == "before" || scenario == "disabled" {
		return f
	}
	metadata := scenario == "metadata" || scenario == "combined" || scenario == "near_limit" || scenario == "many_unknown_fields"
	headers := scenario == "headers" || scenario == "combined" || scenario == "near_limit"
	if metadata {
		f.config = &callercontext.Config{Enabled: true, MetaKey: "bank/context"}
		values := map[string]string{"token": strings.Repeat("a", 2048), "session": "12345678-1234-1234-1234-123456789012"}
		if scenario == "near_limit" {
			values = map[string]string{}
			for i := 0; i < 16; i++ {
				values[fmt.Sprintf("field%d", i)] = strings.Repeat("a", 1950)
			}
		}
		for name := range values {
			field := callercontext.Field{Name: name}
			if kind == "rest" {
				field.Header = "X-Meta-" + name
			}
			f.config.Fields = append(f.config.Fields, field)
		}
		if scenario == "many_unknown_fields" {
			for i := 0; i < 2400; i++ {
				values[fmt.Sprintf("x%d", i)] = ""
			}
		}
		f.meta, _ = json.Marshal(map[string]any{"bank/context": values})
	}
	if headers {
		count, size := 2, 2048
		if scenario == "near_limit" {
			count, size = 4, 8192
		}
		for i := 0; i < count; i++ {
			name := fmt.Sprintf("X-Caller-%d", i)
			f.rules = append(f.rules, callercontext.HeaderMapping{Source: name, Target: fmt.Sprintf("X-Upstream-%d", i)})
			valueSize := size
			if scenario != "near_limit" && i == 1 {
				valueSize = 36
			}
			f.incoming.Set(name, strings.Repeat("a", valueSize))
		}
	}
	return f
}

// BenchmarkForwarding measures only the added in-process request path: selection,
// metadata merging and per-request header copies. It excludes HTTP, database,
// audit, TLS and token verification. "before" is the reference without this path;
// "disabled" includes the current empty-config path. Parallel ns/op is aggregate
// throughput cost, not individual request latency. Fixtures never contain secrets.
func BenchmarkForwarding(b *testing.B) {
	for _, kind := range []string{"mcp", "rest"} {
		for _, scenario := range []string{"before", "disabled", "headers", "metadata", "combined", "near_limit"} {
			f := fixture(kind, scenario)
			if len(f.meta) > callercontext.MaxBytes {
				b.Fatal("invalid fixture size")
			}
			if err := f.config.Validate(kind, nil, false); err != nil {
				b.Fatal(err)
			}
			if err := callercontext.ValidateHeaderMappings(f.rules, nil, f.config, kind); err != nil {
				b.Fatal(err)
			}
			for _, parallel := range []bool{false, true} {
				mode := "serial"
				if parallel {
					mode = "parallel"
				}
				b.Run(kind+"/"+scenario+"/"+mode, func(b *testing.B) {
					base := upstream.WithRelay(context.Background(), upstream.Relay{Meta: json.RawMessage(`{"progressToken":"gateway-progress"}`)})
					invoke := func() {
						req := &http.Request{Header: make(http.Header)}
						ctx := base
						if scenario != "before" {
							selected, dynamic, err := f.config.Prepare(f.meta, kind)
							if err != nil {
								panic(err)
							}
							forwarded, err := callercontext.PrepareHeaders(f.rules, f.incoming)
							if err != nil {
								panic(err)
							}
							if dynamic == nil {
								dynamic = http.Header{}
							}
							for name, values := range forwarded {
								dynamic[name] = values
							}
							ctx = upstream.WithCallerMeta(ctx, selected)
							ctx = callercontext.WithHeaders(ctx, dynamic)
							callercontext.ApplyHeaders(ctx, req)
						}
						runtime.KeepAlive(req)
						runtime.KeepAlive(ctx)
					}
					b.ReportAllocs()
					b.ResetTimer()
					if parallel {
						b.RunParallel(func(pb *testing.PB) {
							for pb.Next() {
								invoke()
							}
						})
					} else {
						for i := 0; i < b.N; i++ {
							invoke()
						}
					}
				})
			}
		}
	}
}

// A byte limit also permits many tiny unknown fields. Measure that separately
// from large token values; unknown fields are parsed but never forwarded.
func BenchmarkUnknownMetadataFields(b *testing.B) {
	for _, kind := range []string{"mcp", "rest"} {
		f := fixture(kind, "many_unknown_fields")
		if len(f.meta) > callercontext.MaxBytes {
			b.Fatal(len(f.meta))
		}
		b.Run(kind, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				selected, headers, err := f.config.Prepare(f.meta, kind)
				if err != nil {
					b.Fatal(err)
				}
				runtime.KeepAlive(selected)
				runtime.KeepAlive(headers)
			}
		})
	}
}
