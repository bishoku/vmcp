package virtual

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestSubscriptionDispatcherBoundsAndRecovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := make(chan time.Time)
	started := make(chan struct{}, 64)
	release := make(chan struct{})
	done := make(chan struct{})
	var claims atomic.Int32
	go func() {
		defer close(done)
		dispatchSubscriptions(ctx, ticks, func(ctx context.Context) func() {
			claims.Add(1)
			return func() {
				started <- struct{}{}
				select {
				case <-release:
				case <-ctx.Done():
				}
			}
		})
	}()
	for range 32 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("worker capacity not filled")
		}
	}
	ticks <- time.Now()
	// A synchronous tick cannot exceed 32 live workers.
	select {
	case <-started:
		t.Fatal("worker limit exceeded")
	case <-time.After(20 * time.Millisecond):
	}
	if claims.Load() != 32 {
		t.Fatalf("claims while full: %d", claims.Load())
	}
	close(release)
	// Released work can be reclaimed on a subsequent discovery tick.
	deadline := time.After(time.Second)
	recovered := false
	for !recovered {
		select {
		case ticks <- time.Now():
		case <-started:
			recovered = true
		case <-deadline:
			t.Fatal("capacity not recovered")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("dispatcher ignored cancellation")
	}
}

func TestSubscriptionDispatcherEmptyPollIsBounded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := make(chan time.Time)
	polled := make(chan struct{}, 10)
	done := make(chan struct{})
	go func() {
		defer close(done)
		dispatchSubscriptions(ctx, ticks, func(context.Context) func() { polled <- struct{}{}; return nil })
	}()
	select {
	case <-polled:
	case <-time.After(time.Second):
		t.Fatal("startup discovery missing")
	}
	for range 3 {
		ticks <- time.Now()
		select {
		case <-polled:
		case <-time.After(time.Second):
			t.Fatal("fallback discovery missing")
		}
	}
	select {
	case <-polled:
		t.Fatal("empty claim spun without a tick")
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("dispatcher ignored cancellation")
	}
}
