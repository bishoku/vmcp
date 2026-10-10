// Package virtual serves virtual MCP servers: per-agent endpoints that expose
// a filtered subset of tools from one or more physical upstreams.
package virtual

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/upstream"
)

// upstreamConn lazily holds the downstream session to one upstream.
type upstreamConn struct {
	mu sync.Mutex
	c  upstream.Conn
}

// Session is one client session on a virtual server. It owns one upstream
// session per upstream the client actually touches (created lazily).
type Session struct {
	id      string
	slug    string
	version string
	// keySlug is the agent key that opened the session ("" on open virtual servers).
	keySlug string

	mu    sync.Mutex
	conns map[string]*upstreamConn
	last  time.Time
	// persisted is when the shared store last saw this session alive.
	persisted time.Time
}

func (s *Session) touch() {
	s.mu.Lock()
	s.last = time.Now()
	s.mu.Unlock()
}

func (s *Session) conn(upstreamID string) *upstreamConn {
	s.mu.Lock()
	defer s.mu.Unlock()
	uc, ok := s.conns[upstreamID]
	if !ok {
		uc = &upstreamConn{}
		s.conns[upstreamID] = uc
	}
	return uc
}

// client returns the initialized upstream client, creating it on first use.
func (s *Session) client(ctx context.Context, up config.Upstream) (upstream.Conn, error) {
	uc := s.conn(up.ID)
	uc.mu.Lock()
	defer uc.mu.Unlock()
	if uc.c != nil {
		return uc.c, nil
	}
	c, err := Dial(ctx, up)
	if err != nil {
		return nil, err
	}
	uc.c = c
	return c, nil
}

// Dial opens and initializes a connection to the upstream, falling back through its
// alternative URLs in order. With fallbacks configured each attempt gets a short budget so a
// black-holed primary does not stall the caller.
func Dial(ctx context.Context, up config.Upstream) (upstream.Conn, error) {
	urls := append([]string{up.URL}, up.FallbackURLs...)
	var lastErr error
	for _, u := range urls {
		c, err := upstream.OpenAuth(up.Transport, u, up.Headers, up.TLS, up.OAuth)
		if err != nil {
			return nil, err
		}
		actx, cancel := ctx, context.CancelFunc(func() {})
		if len(urls) > 1 {
			actx, cancel = context.WithTimeout(ctx, 10*time.Second)
		}
		_, err = c.Initialize(actx)
		cancel()
		if err == nil {
			return c, nil
		}
		lastErr = err
		_ = c.Close(context.Background())
		if ctx.Err() != nil {
			break
		}
	}
	return nil, lastErr
}

// reset drops a client whose upstream session expired, if it is still current.
func (s *Session) reset(upstreamID string, old upstream.Conn) {
	uc := s.conn(upstreamID)
	uc.mu.Lock()
	defer uc.mu.Unlock()
	if uc.c == old {
		uc.c = nil
	}
}

func (s *Session) closeUpstreams() {
	s.mu.Lock()
	conns := make([]*upstreamConn, 0, len(s.conns))
	for _, uc := range s.conns {
		conns = append(conns, uc)
	}
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, uc := range conns {
		uc.mu.Lock()
		if uc.c != nil {
			_ = uc.c.Close(ctx)
		}
		uc.mu.Unlock()
	}
}

// Manager tracks live sessions. Upstream connections are always node-local; with a Store the
// session records are shared, so a node that has never seen a session id can still resume it
// (its upstream sessions are simply re-established on first use).
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	// Store is optional; nil keeps sessions node-local.
	Store SessionStore
}

// touchEvery limits how often a session's liveness is written to the shared store.
const touchEvery = time.Minute

func NewManager() *Manager { return &Manager{sessions: map[string]*Session{}} }

// Create registers a new session bound to a virtual server slug.
func (m *Manager) Create(slug, protocolVersion string) (*Session, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, err
	}
	s := &Session{
		id:      hex.EncodeToString(b[:]),
		slug:    slug,
		version: protocolVersion,
		conns:   map[string]*upstreamConn{},
		last:    time.Now(),
	}
	m.mu.Lock()
	m.sessions[s.id] = s
	m.mu.Unlock()
	return s, nil
}

// Persist records a freshly created session in the shared store.
func (m *Manager) Persist(ctx context.Context, s *Session) error {
	if m.Store == nil {
		return nil
	}
	s.mu.Lock()
	s.persisted = time.Now()
	s.mu.Unlock()
	return m.Store.Save(ctx, Record{ID: s.id, Slug: s.slug, KeySlug: s.keySlug, Version: s.version})
}

// Resolve returns the session, loading it from the shared store when this node does not hold it.
// It returns nil for unknown sessions and an error only if the store is unreachable.
func (m *Manager) Resolve(ctx context.Context, id string) (*Session, error) {
	if id == "" {
		return nil, nil
	}
	if s := m.Get(id); s != nil {
		return s, nil
	}
	if m.Store == nil {
		return nil, nil
	}
	rec, ok, err := m.Store.Load(ctx, id)
	if err != nil || !ok {
		return nil, err
	}
	s := &Session{id: rec.ID, slug: rec.Slug, keySlug: rec.KeySlug, version: rec.Version, conns: map[string]*upstreamConn{}, last: time.Now(), persisted: rec.LastSeen}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing := m.sessions[id]; existing != nil { // lost a race with a concurrent request
		return existing, nil
	}
	m.sessions[id] = s
	return s, nil
}

// Touch refreshes the shared liveness of a session at most once per touchEvery.
func (m *Manager) Touch(ctx context.Context, s *Session) {
	if m.Store == nil {
		return
	}
	s.mu.Lock()
	due := time.Since(s.persisted) >= touchEvery
	if due {
		s.persisted = time.Now()
	}
	s.mu.Unlock()
	if due {
		_ = m.Store.Touch(ctx, s.id) // best effort: a missed touch only shortens the session's life
	}
}

// End terminates a session everywhere: this node's connections and the shared record.
func (m *Manager) End(ctx context.Context, id string) bool {
	if m.Store != nil {
		_ = m.Store.Delete(ctx, id)
	}
	return m.Delete(id)
}

func (m *Manager) Get(id string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

// Delete removes a session and closes its upstream sessions.
func (m *Manager) Delete(id string) bool {
	m.mu.Lock()
	s, ok := m.sessions[id]
	delete(m.sessions, id)
	m.mu.Unlock()
	if ok {
		go s.closeUpstreams()
	}
	return ok
}

// Len returns the number of live sessions.
func (m *Manager) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// Reap evicts sessions idle longer than ttl from this node (closing their upstream sessions).
// The shared record stays until it is purged, so another request may resume the session.
// Returns how many were evicted.
func (m *Manager) Reap(ttl time.Duration) int {
	cutoff := time.Now().Add(-ttl)
	var stale []string
	m.mu.RLock()
	for id, s := range m.sessions {
		s.mu.Lock()
		if s.last.Before(cutoff) {
			stale = append(stale, id)
		}
		s.mu.Unlock()
	}
	m.mu.RUnlock()
	for _, id := range stale {
		m.Delete(id)
	}
	return len(stale)
}

// RunReaper reaps idle sessions periodically until ctx is done.
func (m *Manager) RunReaper(ctx context.Context, ttl time.Duration) {
	t := time.NewTicker(ttl / 2)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.Reap(ttl)
			if m.Store != nil {
				ctx2, cancel := context.WithTimeout(ctx, 10*time.Second)
				_, _ = m.Store.Purge(ctx2, ttl)
				cancel()
			}
		}
	}
}
