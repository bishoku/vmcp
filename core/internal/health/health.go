// Package health probes upstreams in the background and publishes what this node sees,
// so operators can tell a dead upstream from a misconfigured one before an agent does.
package health

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/resilience"
	"github.com/bishoku/vmcp/core/internal/upstream"
)

const probeTimeout = 10 * time.Second

// Snapshots provides the current configuration.
type Snapshots interface{ Snapshot() *config.Snapshot }

// Checker probes every enabled upstream each Interval. Results go to Postgres (one row per
// upstream and node) and into the circuit breakers, so a recovered upstream is let back in
// without waiting for an agent to be the guinea pig.
type Checker struct {
	Pool     *pgxpool.Pool
	Snap     Snapshots
	Breakers *resilience.Registry
	Node     string
	Interval time.Duration
	Log      *slog.Logger
}

// Run checks until ctx is done. It does nothing when Interval is not positive.
func (c *Checker) Run(ctx context.Context) {
	if c.Interval <= 0 {
		return
	}
	t := time.NewTicker(c.Interval)
	defer t.Stop()
	for {
		c.CheckOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// CheckOnce probes all upstreams concurrently and records the results.
func (c *Checker) CheckOnce(ctx context.Context) {
	var wg sync.WaitGroup
	for _, up := range c.Snap.Snapshot().Upstreams {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.check(ctx, up)
		}()
	}
	wg.Wait()
}

func (c *Checker) check(ctx context.Context, up config.Upstream) {
	url, took, err := Probe(ctx, up)
	br := c.Breakers.Get(up.ID)
	state, breaker := "healthy", "closed"
	var msg *string
	if err != nil {
		state = "unhealthy"
		s := err.Error()
		msg = &s
		if br != nil && ctx.Err() == nil {
			br.Failure()
		}
	} else if br != nil {
		br.Success()
	}
	if br != nil {
		breaker = br.State().String()
	}
	if ctx.Err() != nil {
		return
	}
	if _, err := c.Pool.Exec(ctx, `
		INSERT INTO upstream_health (upstream_id, node, state, breaker, url, latency_ms, error, checked_at)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (upstream_id, node) DO UPDATE
		SET state = $3, breaker = $4, url = $5, latency_ms = $6, error = $7, checked_at = now()`,
		up.ID, c.Node, state, breaker, url, took.Milliseconds(), msg); err != nil && c.Log != nil {
		c.Log.Warn("cannot record upstream health", "upstream", up.Slug, "err", err)
	}
}

// Probe connects to the upstream (trying its URLs in order) and returns the URL that answered.
func Probe(ctx context.Context, up config.Upstream) (url string, took time.Duration, err error) {
	urls := append([]string{up.URL}, up.FallbackURLs...)
	for _, u := range urls {
		start := time.Now()
		pctx, cancel := context.WithTimeout(ctx, probeTimeout)
		var conn upstream.Conn
		if conn, err = upstream.Open(up.Transport, u, up.Headers, up.TLS); err == nil {
			_, err = conn.Initialize(pctx)
			_ = conn.Close(pctx)
		}
		cancel()
		if err == nil {
			return u, time.Since(start), nil
		}
	}
	return urls[len(urls)-1], 0, err
}
