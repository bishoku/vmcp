package resilience

import (
	"testing"
	"time"
)

func newTest(threshold int) (*Breaker, *time.Time) {
	t := time.Unix(0, 0)
	return &Breaker{Threshold: threshold, Cooldown: 10 * time.Second, now: func() time.Time { return t }}, &t
}

func TestBreakerCycle(t *testing.T) {
	b, clock := newTest(3)
	for i := 0; i < 2; i++ {
		b.Failure()
	}
	if !b.Allow() || b.State() != Closed {
		t.Fatal("must stay closed below the threshold")
	}
	b.Success() // resets the streak
	b.Failure()
	b.Failure()
	if b.State() != Closed {
		t.Fatal("success must reset consecutive failures")
	}
	b.Failure()
	if b.State() != Open || b.Allow() {
		t.Fatal("must open at the threshold and refuse calls")
	}

	*clock = clock.Add(11 * time.Second)
	if !b.Allow() {
		t.Fatal("first call after cooldown is the probe")
	}
	if b.Allow() {
		t.Fatal("only one probe at a time")
	}
	b.Failure()
	if b.State() != Open || b.Allow() {
		t.Fatal("failed probe reopens")
	}

	*clock = clock.Add(11 * time.Second)
	if !b.Allow() {
		t.Fatal("probe again")
	}
	b.Success()
	if b.State() != Closed || !b.Allow() || !b.Allow() {
		t.Fatal("successful probe closes")
	}
}

func TestLostProbeIsReplaced(t *testing.T) {
	b, clock := newTest(1)
	b.Failure()
	*clock = clock.Add(11 * time.Second)
	if !b.Allow() {
		t.Fatal("probe")
	}
	*clock = clock.Add(11 * time.Second) // the probe never reported back
	if !b.Allow() {
		t.Fatal("a lost probe must not wedge the breaker")
	}
}

func TestDisabledRegistry(t *testing.T) {
	if NewRegistry(0, time.Second).Get("x") != nil {
		t.Fatal("threshold 0 disables breaking")
	}
}
