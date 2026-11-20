package upstream

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/bishoku/vmcp/core/internal/mcp"
)

type preferenceKey struct {
	url        string
	generation [32]byte
}
type preference struct {
	modern  bool
	expires time.Time
	pending chan struct{}
}

// PreferenceCache belongs to one data-plane manager. Only wire revision verdicts
// are shared. Session IDs, client capabilities and connections stay with callers.
type PreferenceCache struct {
	mu      sync.Mutex
	entries map[preferenceKey]*preference
	// Clock defaults to wall time and can be set before the manager starts.
	Clock func() time.Time
}

func (p *PreferenceCache) now() time.Time {
	if p.Clock != nil {
		return p.Clock()
	}
	return time.Now()
}

func (p *PreferenceCache) resolve(ctx context.Context, c *Client, key preferenceKey) (*preference, error) {
	for {
		p.mu.Lock()
		entry := p.entries[key]
		if entry != nil && entry.pending != nil {
			wait := entry.pending
			p.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if entry != nil && p.now().Before(entry.expires) {
			p.mu.Unlock()
			return entry, nil
		}
		if len(p.entries) >= 4096 && entry == nil {
			for k, v := range p.entries {
				if v.pending == nil {
					delete(p.entries, k)
					break
				}
			}
			if len(p.entries) >= 4096 {
				p.mu.Unlock()
				return nil, errors.New("upstream discovery capacity exhausted")
			}
		}
		if p.entries == nil {
			p.entries = make(map[preferenceKey]*preference)
		}
		wait := make(chan struct{})
		p.entries[key] = &preference{pending: wait}
		p.mu.Unlock()
		// Discover on an isolated client. Revalidation must never change the wire
		// version, request IDs or capabilities of a connection with leased requests.
		probe := &Client{url: c.url, headers: c.headers, http: c.http}
		d, err := probe.discover(ctx)
		p.mu.Lock()
		if err != nil {
			delete(p.entries, key)
		} else {
			entry = &preference{modern: d != nil, expires: p.now().Add(5 * time.Minute)}
			p.entries[key] = entry
		}
		close(wait)
		p.mu.Unlock()
		return entry, err
	}
}

func (p *PreferenceCache) PrepareInvocation(ctx context.Context, conn Conn, url string, generation [32]byte) error {
	c, ok := conn.(*Client)
	if !ok || !mcp.ModernEnabled() {
		_, err := conn.Initialize(ctx)
		return err
	}
	c.preferences = p
	c.preference = preferenceKey{url, generation}
	v, err := p.resolve(ctx, c, c.preference)
	if err != nil {
		return err
	}
	c.verdict.Store(v)
	if v.modern {
		c.protocolVersion = mcp.ProtocolVersion20260728
		return nil
	}
	_, err = c.initializeLegacy(ctx)
	if err != nil {
		c.forgetPreference()
	}
	return err
}

func (c *Client) forgetPreference() {
	p := c.preferences
	if p == nil {
		return
	}
	c.retired.Store(true)
	p.mu.Lock()
	if p.entries[c.preference] == c.verdict.Load() {
		delete(p.entries, c.preference)
	}
	p.mu.Unlock()
}

// NeedsDiscovery preserves a live session when its wire revision still matches.
// The session owner retires changed connections after active leases finish.
func (c *Client) NeedsDiscovery(ctx context.Context) (bool, error) {
	if c.retired.Load() {
		return true, nil
	}
	p := c.preferences
	if p == nil {
		return false, nil
	}
	old := c.verdict.Load()
	if p.now().Before(old.expires) {
		return false, nil
	}
	v, err := p.resolve(ctx, c, c.preference)
	if err != nil {
		return false, err
	}
	if v.modern != old.modern {
		return true, nil
	}
	c.verdict.Store(v)
	return c.retired.Load(), nil
}
