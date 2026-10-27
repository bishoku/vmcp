package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// SemanticCache caches search results by query and role hash.
type SemanticCache interface {
	Get(ctx context.Context, query SearchQuery) ([]SearchResult, bool)
	Set(ctx context.Context, query SearchQuery, results []SearchResult, ttl time.Duration)
	Invalidate(ctx context.Context, toolID string)
}

// BuildCacheKey generates `mcp:cache:query:<sha256(user_query + role + domain)>`.
func BuildCacheKey(q SearchQuery) string {
	raw := fmt.Sprintf("%s|%s|%s|%d", q.Query, q.UserRole, q.Domain, q.Limit)
	h := sha256.Sum256([]byte(raw))
	return "mcp:cache:query:" + hex.EncodeToString(h[:])
}

// MemorySemanticCache is a high-speed in-memory L1 cache with TTL.
type MemorySemanticCache struct {
	mu    sync.RWMutex
	items map[string]memoryCacheEntry
}

type memoryCacheEntry struct {
	results   []SearchResult
	expiresAt time.Time
}

// NewMemorySemanticCache creates an in-memory semantic cache.
func NewMemorySemanticCache() *MemorySemanticCache {
	c := &MemorySemanticCache{
		items: make(map[string]memoryCacheEntry),
	}
	// Background cleanup of expired entries
	go c.cleanupLoop()
	return c
}

func (c *MemorySemanticCache) Get(_ context.Context, q SearchQuery) ([]SearchResult, bool) {
	key := BuildCacheKey(q)
	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()

	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.results, true
}

func (c *MemorySemanticCache) Set(_ context.Context, q SearchQuery, results []SearchResult, ttl time.Duration) {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	key := BuildCacheKey(q)
	c.mu.Lock()
	c.items[key] = memoryCacheEntry{
		results:   results,
		expiresAt: time.Now().Add(ttl),
	}
	c.mu.Unlock()
}

func (c *MemorySemanticCache) Invalidate(_ context.Context, toolID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, v := range c.items {
		for _, res := range v.results {
			if res.Tool.ID == toolID {
				delete(c.items, k)
				break
			}
		}
	}
}

func (c *MemorySemanticCache) cleanupLoop() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		c.mu.Lock()
		for k, v := range c.items {
			if now.After(v.expiresAt) {
				delete(c.items, k)
			}
		}
		c.mu.Unlock()
	}
}

// RedisSemanticCache connects to Redis Cluster/Instance for distributed L1 semantic caching.
type RedisSemanticCache struct {
	client redis.UniversalClient
}

// NewRedisSemanticCache creates a Redis-backed semantic cache.
func NewRedisSemanticCache(client redis.UniversalClient) *RedisSemanticCache {
	return &RedisSemanticCache{client: client}
}

func (r *RedisSemanticCache) Get(ctx context.Context, q SearchQuery) ([]SearchResult, bool) {
	if r.client == nil {
		return nil, false
	}
	key := BuildCacheKey(q)
	raw, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}
	var results []SearchResult
	if err := json.Unmarshal(raw, &results); err != nil {
		return nil, false
	}
	return results, true
}

func (r *RedisSemanticCache) Set(ctx context.Context, q SearchQuery, results []SearchResult, ttl time.Duration) {
	if r.client == nil {
		return
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	key := BuildCacheKey(q)
	raw, err := json.Marshal(results)
	if err != nil {
		return
	}
	_ = r.client.Set(ctx, key, raw, ttl).Err()
}

func (r *RedisSemanticCache) Invalidate(ctx context.Context, _ string) {
	// PubSub cache eviction can publish to `mcp:catalog:events`
	if r.client != nil {
		_ = r.client.Publish(ctx, "mcp:catalog:events", `{"event":"CACHE_FLUSH"}`).Err()
	}
}
