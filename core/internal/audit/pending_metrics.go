package audit

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PendingMetrics samples the database independently of metric readers. A failed
// or timed-out refresh removes the sample until another refresh succeeds.
type PendingMetrics struct{ latest atomic.Pointer[PendingStatus] }

func StartPendingMetrics(ctx context.Context, pool *pgxpool.Pool) (*PendingMetrics, func()) {
	return samplePendingMetrics(ctx, func(c context.Context) (PendingStatus, error) { return Pending(c, pool) })
}
func samplePendingMetrics(ctx context.Context, load func(context.Context) (PendingStatus, error)) (*PendingMetrics, func()) {
	refresh, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	m := new(PendingMetrics)
	go func() { defer close(done); m.run(refresh, load) }()
	return m, func() { stop(); <-done }
}

func (m *PendingMetrics) run(ctx context.Context, load func(context.Context) (PendingStatus, error)) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		query, cancel := context.WithTimeout(ctx, time.Second)
		sample, err := load(query)
		cancel()
		if err == nil {
			m.latest.Store(&sample)
		} else {
			m.latest.Store(nil)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (m *PendingMetrics) AppendTo(gauges map[string]float64) {
	if s := m.latest.Load(); s != nil {
		gauges["audit_incomplete"] = float64(s.Incomplete)
		gauges["audit_oldest_incomplete_seconds"] = s.OldestIncompleteAgeSeconds
	}
}
