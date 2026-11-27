package audit

import (
	"context"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/telemetry"
)

func TestMetricReadersNeverRefreshPending(t *testing.T) {
	var queries atomic.Int64
	cache, stop := samplePendingMetrics(t.Context(), func(context.Context) (PendingStatus, error) {
		queries.Add(1)
		return PendingStatus{Incomplete: 7}, nil
	})
	defer stop()
	deadline := time.Now().Add(time.Second)
	for cache.latest.Load() == nil {
		if time.Now().After(deadline) {
			t.Fatal("initial refresh timed out")
		}
		time.Sleep(time.Millisecond)
	}
	r := telemetry.New()
	r.SetGauges(func() map[string]float64 { values := map[string]float64{}; cache.AppendTo(values); return values })
	for range 100 {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/metrics", nil))
	}
	data, err := r.Produce(t.Context())
	if err != nil || len(data) == 0 || queries.Load() != 1 {
		t.Fatalf("queries=%d, err=%v", queries.Load(), err)
	}
}
