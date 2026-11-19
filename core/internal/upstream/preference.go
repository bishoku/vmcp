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

var preferences = struct {
	sync.Mutex
	entries map[preferenceKey]preference
}{entries: make(map[preferenceKey]preference)}

// PrepareInvocation only needs the wire revision. Administrative discovery still
// reads capabilities and catalog metadata from the authenticated server.
func PrepareInvocation(ctx context.Context, conn Conn, url string, generation [32]byte) error {
	c, ok := conn.(*Client)
	if !ok || !mcp.ModernEnabled() {
		_, err := conn.Initialize(ctx)
		return err
	}
	key := preferenceKey{url, generation}
	for {
		preferences.Lock()
		entry, ok := preferences.entries[key]
		if entry.pending != nil {
			preferences.Unlock()
			select {
			case <-entry.pending:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if ok && time.Now().Before(entry.expires) {
			preferences.Unlock()
			c.preference, c.preferenceExpiry = key, entry.expires
			if entry.modern {
				c.protocolVersion = mcp.ProtocolVersion20260728
				return nil
			}
			_, err := c.initializeLegacy(ctx)
			if err != nil {
				c.forgetPreference()
			}
			return err
		}
		if len(preferences.entries) >= 4096 {
			for k, v := range preferences.entries {
				if v.pending == nil {
					delete(preferences.entries, k)
					break
				}
			}
		}
		if len(preferences.entries) >= 4096 {
			preferences.Unlock()
			return errors.New("upstream discovery capacity exhausted")
		}

		pending := make(chan struct{})
		preferences.entries[key] = preference{pending: pending}
		preferences.Unlock()
		d, err := c.discover(ctx)
		modern := d != nil
		if err == nil && !modern {
			_, err = c.initializeLegacy(ctx)
		}
		preferences.Lock()
		if err != nil {
			delete(preferences.entries, key)
		} else {
			c.preference, c.preferenceExpiry = key, time.Now().Add(5*time.Minute)
			preferences.entries[key] = preference{modern: modern, expires: c.preferenceExpiry}
		}
		close(pending)
		preferences.Unlock()
		return err
	}
}

// A failed dispatch may have executed. Only retire the discovery hint for the
// next independent connection. Never resend the failed operation here.
func (c *Client) forgetPreference() {
	if c.preferenceExpiry.IsZero() {
		return
	}
	c.retired.Store(true)
	preferences.Lock()
	defer preferences.Unlock()
	if preferences.entries[c.preference].expires.Equal(c.preferenceExpiry) {
		delete(preferences.entries, c.preference)
	}
}

// NeedsDiscovery signals a connection fault or explicit revision rejection.
func (c *Client) NeedsDiscovery() bool { return c.retired.Load() }
