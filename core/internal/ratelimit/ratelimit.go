// Package ratelimit limits calls per minute with token buckets. A bucket holds up to `limit`
// tokens and refills at limit/60 per second, so short bursts up to the limit are fine.
//
// Without Redis every node counts on its own, so the effective limit of a deployment is up to
// N times the configured one. With Redis the count is shared by all nodes.
package ratelimit

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter takes one token from the bucket named key, which holds limit tokens per minute.
type Limiter interface {
	Allow(ctx context.Context, key string, limit int) (ok bool, retryAfter time.Duration, err error)
}

// Local is an in-memory limiter, one bucket per key.
type Local struct {
	now     func() time.Time
	mu      sync.Mutex
	buckets map[string]*bucket
	swept   time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

func NewLocal() *Local { return &Local{now: time.Now, buckets: map[string]*bucket{}} }

func (l *Local) Allow(_ context.Context, key string, limit int) (bool, time.Duration, error) {
	now := l.now()
	rate := float64(limit) / 60 // tokens per second
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweep(now)
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(limit), last: now}
		l.buckets[key] = b
	}
	b.tokens = min(float64(limit), b.tokens+now.Sub(b.last).Seconds()*rate)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0, nil
	}
	return false, time.Duration((1 - b.tokens) / rate * float64(time.Second)), nil
}

// sweep drops buckets idle for over two minutes: a bucket idle that long is full again, so a
// fresh one is equivalent.
func (l *Local) sweep(now time.Time) {
	if now.Sub(l.swept) < time.Minute {
		return
	}
	l.swept = now
	for k, b := range l.buckets {
		if now.Sub(b.last) > 2*time.Minute {
			delete(l.buckets, k)
		}
	}
}

// tokenBucket runs atomically in Redis and uses Redis' own clock, so nodes with skewed clocks
// still agree. KEYS[1]=bucket, ARGV[1]=limit per minute. Returns {allowed, retryAfterMs}.
var tokenBucket = redis.NewScript(`
local limit = tonumber(ARGV[1])
local rate = limit / 60000
local t = redis.call('TIME')
local now = t[1] * 1000 + math.floor(t[2] / 1000)
local d = redis.call('HMGET', KEYS[1], 'tokens', 'ts')
local tokens = tonumber(d[1])
local ts = tonumber(d[2])
if tokens == nil then tokens = limit; ts = now end
tokens = math.min(limit, tokens + math.max(0, now - ts) * rate)
local allowed = 0
local retry = 0
if tokens >= 1 then
  tokens = tokens - 1
  allowed = 1
else
  retry = math.ceil((1 - tokens) / rate)
end
redis.call('HSET', KEYS[1], 'tokens', tokens, 'ts', now)
redis.call('PEXPIRE', KEYS[1], 120000)
return {allowed, retry}
`)

// Redis shares buckets between nodes.
type Redis struct {
	Client  redis.Scripter
	Prefix  string
	Timeout time.Duration
}

func (r *Redis) Allow(ctx context.Context, key string, limit int) (bool, time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	res, err := tokenBucket.Run(ctx, r.Client, []string{r.Prefix + key}, limit).Int64Slice()
	if err != nil {
		return false, 0, err
	}
	return res[0] == 1, time.Duration(res[1]) * time.Millisecond, nil
}

// FailOpen asks Primary and, when it errors (Redis unreachable), falls back to Fallback so
// limits keep applying per node instead of blocking or disappearing. Errors are logged at
// most once a minute.
type FailOpen struct {
	Primary, Fallback Limiter
	Log               *slog.Logger
	Strict            bool
	Failures          atomic.Int64
	Degraded          atomic.Bool

	mu     sync.Mutex
	logged time.Time
}

func (f *FailOpen) Allow(ctx context.Context, key string, limit int) (bool, time.Duration, error) {
	ok, retry, err := f.Primary.Allow(ctx, key, limit)
	if err == nil {
		f.Degraded.Store(false)
		return ok, retry, nil
	}
	f.Failures.Add(1)
	f.Degraded.Store(true)
	if f.Strict {
		return false, time.Second, nil
	}
	f.mu.Lock()
	if f.Log != nil && time.Since(f.logged) > time.Minute {
		f.logged = time.Now()
		f.Log.Warn("shared rate limiter unavailable, limiting per node", "err", err)
	}
	f.mu.Unlock()
	return f.Fallback.Allow(ctx, key, limit)
}

// NewRedis builds a Limiter from a redis:// URL, falling back to per-node limits when Redis
// cannot be reached.
func NewRedis(url string, log *slog.Logger) (Limiter, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	return &FailOpen{
		Primary:  &Redis{Client: redis.NewClient(opt), Prefix: "vmcp:rl:", Timeout: 250 * time.Millisecond},
		Fallback: NewLocal(),
		Log:      log,
	}, nil
}

// Check refreshes dependency health without consuming a caller's quota.
func (f *FailOpen) Check(ctx context.Context) error {
	r, ok := f.Primary.(*Redis)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	err := r.Client.Eval(ctx, "return 1", nil).Err()
	f.Degraded.Store(err != nil)
	return err
}
