package telemetry

import (
	"runtime"
	"runtime/metrics"
)

func runtimeMetrics() map[string]float64 {
	samples := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}, {Name: "/gc/heap/allocs:bytes"}, {Name: "/gc/cycles/total:gc-cycles"}}
	metrics.Read(samples)
	return map[string]float64{"go_goroutines": float64(runtime.NumGoroutine()), "go_memstats_heap_alloc_bytes": float64(samples[0].Value.Uint64()), "go_memstats_alloc_bytes_total": float64(samples[1].Value.Uint64()), "go_gc_cycles_total": float64(samples[2].Value.Uint64())}
}
