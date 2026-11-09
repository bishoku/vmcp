package telemetry

import (
	"fmt"
	"io"
	"runtime"
	"runtime/metrics"
)

func runtimeMetrics(w io.Writer) {
	samples := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}, {Name: "/gc/heap/allocs:bytes"}, {Name: "/gc/cycles/total:gc-cycles"}}
	metrics.Read(samples)
	fmt.Fprintf(w, "# TYPE go_goroutines gauge\ngo_goroutines %d\n", runtime.NumGoroutine())
	names := []string{"go_memstats_heap_alloc_bytes", "go_memstats_alloc_bytes_total", "go_gc_cycles_total"}
	for i, s := range samples {
		kind := "counter"
		if i == 0 {
			kind = "gauge"
		}
		fmt.Fprintf(w, "# TYPE %s %s\n%s %d\n", names[i], kind, names[i], s.Value.Uint64())
	}
}
