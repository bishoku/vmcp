// Package lifecycle coordinates HTTP admission and graceful draining.
package lifecycle

import (
	"context"
	"net/http"
	"sync"
)

type Gate struct {
	mu       sync.Mutex
	draining bool
	active   sync.WaitGroup
}

func (g *Gate) Draining() bool { g.mu.Lock(); defer g.mu.Unlock(); return g.draining }
func (g *Gate) Stop()          { g.mu.Lock(); g.draining = true; g.mu.Unlock() }
func (g *Gate) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		if g.draining && r.URL.Path != "/healthz" && r.URL.Path != "/readyz" {
			g.mu.Unlock()
			http.Error(w, "server draining", 503)
			return
		}
		if g.draining {
			g.mu.Unlock()
			next.ServeHTTP(w, r)
			return
		}
		g.active.Add(1)
		g.mu.Unlock()
		defer g.active.Done()
		next.ServeHTTP(w, r)
	})
}

// Wait must be called after Stop; no new work can enter the drained gate.
func (g *Gate) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() { g.active.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
