// Package resilience protects the gateway and its upstreams from repeated failures.
package resilience

import (
	"sync"
	"time"
)

// State of a circuit breaker.
type State int

const (
	Closed   State = iota // calls flow
	Open                  // calls are refused until the cooldown passes
	HalfOpen              // one probe call decides whether to close again
)

func (s State) String() string {
	return [...]string{"closed", "open", "half_open"}[s]
}

// Breaker trips after Threshold consecutive failures and probes again after Cooldown.
// It is per node: each data-plane node judges an upstream by what it observes itself.
type Breaker struct {
	Threshold int
	Cooldown  time.Duration
	now       func() time.Time

	mu         sync.Mutex
	state      State
	failures   int
	since      time.Time // when the breaker opened or the current probe started
	probing    bool
	generation uint64
}

// Allow reports whether a call may proceed. In half-open state only one probe is let through;
// a probe that never reports back is replaced after another Cooldown.
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.allowLocked()
}

func (b *Breaker) allowLocked() bool {
	now := b.now()
	switch b.state {
	case Open:
		if now.Sub(b.since) < b.Cooldown {
			return false
		}
		b.generation++
		b.state, b.probing, b.since = HalfOpen, true, now
		return true
	case HalfOpen:
		if b.probing && now.Sub(b.since) < b.Cooldown {
			return false
		}
		b.generation++
		b.probing, b.since = true, now
		return true
	}
	return true
}

// Success records a call that reached the upstream.
func (b *Breaker) Success() {
	b.mu.Lock()
	b.generation++
	b.state, b.failures, b.probing = Closed, 0, false
	b.mu.Unlock()
}

// Failure records a call that could not reach the upstream (transport error, timeout).
func (b *Breaker) Failure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures++
	if b.state == HalfOpen || b.failures >= b.Threshold {
		b.generation++
		b.state, b.since, b.probing = Open, b.now(), false
	}
}

// State returns the current state without changing it.
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == Open && b.now().Sub(b.since) >= b.Cooldown {
		return HalfOpen // would be probed by the next call
	}
	return b.state
}

// Registry hands out one breaker per upstream id. A nil *Registry disables breaking.
type Registry struct {
	threshold int
	cooldown  time.Duration
	mu        sync.Mutex
	m         map[string]*Breaker
}

// NewRegistry returns nil when threshold <= 0 (breaking disabled).
func NewRegistry(threshold int, cooldown time.Duration) *Registry {
	if threshold <= 0 {
		return nil
	}
	return &Registry{threshold: threshold, cooldown: cooldown, m: map[string]*Breaker{}}
}

// Get returns the breaker of an upstream, or nil when breaking is disabled.
func (r *Registry) Get(upstreamID string) *Breaker {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.m[upstreamID]
	if !ok {
		b = &Breaker{Threshold: r.threshold, Cooldown: r.cooldown, now: time.Now}
		r.m[upstreamID] = b
	}
	return b
}

// CountStates aggregates without exposing unbounded upstream identifiers as labels.
func (r *Registry) CountStates() map[State]int {
	counts := map[State]int{}
	if r == nil {
		return counts
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, b := range r.m {
		counts[b.State()]++
	}
	return counts
}

// Acquire returns an idempotent completion bound to this breaker generation.
// Ignored probes release their slot; late results cannot finish a replacement.
func (b *Breaker) Acquire() (func(Outcome), bool) {
	if b == nil {
		return func(Outcome) {}, true
	}
	b.mu.Lock()
	if !b.allowLocked() {
		b.mu.Unlock()
		return nil, false
	}
	generation, probe := b.generation, b.state == HalfOpen
	b.mu.Unlock()
	var once sync.Once
	return func(outcome Outcome) {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if generation != b.generation {
				return
			}
			switch outcome {
			case Ignore:
				if probe {
					b.probing = false
					b.generation++
				}
			case Reached:
				b.state, b.failures, b.probing = Closed, 0, false
				b.generation++
			case Failed:
				b.failures++
				if probe || b.failures >= b.Threshold {
					b.state, b.since, b.probing = Open, b.now(), false
					b.generation++
				}
			}
		})
	}, true
}

// OldestProbeAge exposes a bounded aggregate without upstream identifier labels.
func (r *Registry) OldestProbeAge() time.Duration {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var age time.Duration
	for _, b := range r.m {
		b.mu.Lock()
		if b.state == HalfOpen && b.probing {
			age = max(age, b.now().Sub(b.since))
		}
		b.mu.Unlock()
	}
	return age
}
