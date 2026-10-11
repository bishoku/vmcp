package ratelimit

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestLocalBucket(t *testing.T) {
	clock := time.Unix(1000, 0)
	l := NewLocal()
	l.now = func() time.Time { return clock }
	ctx := context.Background()

	for i := 0; i < 60; i++ {
		if ok, _, _ := l.Allow(ctx, "k", 60); !ok {
			t.Fatalf("call %d should pass: a bucket starts full", i+1)
		}
	}
	ok, retry, _ := l.Allow(ctx, "k", 60)
	if ok || retry < 900*time.Millisecond || retry > 1100*time.Millisecond {
		t.Fatalf("61st call must be refused with ~1s retry, got ok=%v retry=%v", ok, retry)
	}
	clock = clock.Add(2 * time.Second) // 60/min = 1 token per second
	for i := 0; i < 2; i++ {
		if ok, _, _ := l.Allow(ctx, "k", 60); !ok {
			t.Fatalf("refill call %d should pass", i+1)
		}
	}
	if ok, _, _ := l.Allow(ctx, "k", 60); ok {
		t.Fatal("only two tokens were refilled")
	}
	if ok, _, _ := l.Allow(ctx, "other", 60); !ok {
		t.Fatal("keys must not share buckets")
	}
}

func TestLocalSweepsIdleBuckets(t *testing.T) {
	clock := time.Unix(1000, 0)
	l := NewLocal()
	l.now = func() time.Time { return clock }
	l.Allow(context.Background(), "k", 5)
	clock = clock.Add(5 * time.Minute)
	l.Allow(context.Background(), "other", 5)
	if _, ok := l.buckets["k"]; ok {
		t.Fatal("idle bucket should have been swept")
	}
}

type failing struct{}

func (failing) Allow(context.Context, string, int) (bool, time.Duration, error) {
	return false, 0, errors.New("redis down")
}

func TestFailOpenFallsBackToLocal(t *testing.T) {
	f := &FailOpen{Primary: failing{}, Fallback: NewLocal()}
	for i := 0; i < 2; i++ {
		if ok, _, err := f.Allow(context.Background(), "k", 2); !ok || err != nil {
			t.Fatalf("call %d should pass through the fallback", i+1)
		}
	}
	if ok, _, _ := f.Allow(context.Background(), "k", 2); ok {
		t.Fatal("the fallback must still enforce the limit")
	}
}

// Needs a Redis: VMCP_TEST_REDIS_URL=redis://localhost:6379/0
func TestRedisSharedBetweenNodes(t *testing.T) {
	url := os.Getenv("VMCP_TEST_REDIS_URL")
	if url == "" {
		if os.Getenv("VMCP_REQUIRE_INTEGRATION") == "1" {
			t.Fatal("required Redis integration URL is missing")
		}
		t.Skip("VMCP_TEST_REDIS_URL not set")
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "vmcp:test:" + time.Now().Format("150405.000") + ":"
	nodeA := &Redis{Client: redis.NewClient(opt), Prefix: prefix, Timeout: time.Second}
	nodeB := &Redis{Client: redis.NewClient(opt), Prefix: prefix, Timeout: time.Second}
	ctx := context.Background()

	// 10 per minute shared: 5 on each node exhaust it together.
	for i := 0; i < 5; i++ {
		for _, n := range []*Redis{nodeA, nodeB} {
			if ok, _, err := n.Allow(ctx, "k", 10); err != nil || !ok {
				t.Fatalf("call %d: ok=%v err=%v", i, ok, err)
			}
		}
	}
	ok, retry, err := nodeA.Allow(ctx, "k", 10)
	if err != nil || ok || retry <= 0 || retry > 7*time.Second {
		t.Fatalf("shared bucket must be empty for both nodes: ok=%v retry=%v err=%v", ok, retry, err)
	}
	if ok, _, _ := nodeB.Allow(ctx, "k", 10); ok {
		t.Fatal("node B must see the same empty bucket")
	}
	if ok, _, _ := nodeA.Allow(ctx, "another-key", 10); !ok {
		t.Fatal("buckets are per key")
	}
}

func TestStrictQuotaDeniesDuringRedisOutage(t *testing.T) {
	f := &FailOpen{Primary: failing{}, Fallback: NewLocal(), Strict: true}
	ok, retry, err := f.Allow(context.Background(), "principal", 10)
	if ok || err != nil || retry <= 0 || !f.Degraded.Load() || f.Failures.Load() != 1 {
		t.Fatalf("strict quota escaped: %v %v %v", ok, retry, err)
	}
}
