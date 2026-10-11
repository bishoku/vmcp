// Package virtual serves virtual MCP servers: per-agent endpoints that expose
// a filtered subset of tools from one or more physical upstreams.
package virtual

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/upstream"
)

// upstreamConn lazily holds the downstream session to one upstream.
type upstreamConn struct {
	mu      sync.Mutex
	current *cachedClient
}

type cachedClient struct {
	conn       upstream.Conn
	generation [32]byte
	active     int
	retired    bool
	closed     bool
}

// retireLocked prevents reuse; active calls release their lease before closure.
func (uc *upstreamConn) retireLocked() {
	if c := uc.current; c != nil {
		c.retired = true
		uc.current = nil
		uc.closeLocked(c)
	}
}
func (uc *upstreamConn) closeLocked(c *cachedClient) {
	if c.retired && c.active == 0 && !c.closed {
		c.closed = true
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = c.conn.Close(ctx)
		}()
	}
}

// Session is one client session on a virtual server. It owns one upstream
// session per upstream the client actually touches (created lazily).
type Session struct {
	id                 string
	slug               string
	version            string
	initialized        bool
	clientCapabilities json.RawMessage
	// keySlug is the agent key that opened the session ("" on open virtual servers).
	keySlug string
	owner   Owner
	ended   atomic.Bool

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
func (s *Session) client(ctx context.Context, up config.Upstream) (upstream.Conn, func(), error) {
	uc := s.conn(up.ID)
	uc.mu.Lock()
	defer uc.mu.Unlock()
	if s.ended.Load() {
		return nil, nil, errors.New("session ended")
	}
	generation := up.Fingerprint()
	if uc.current != nil && uc.current.generation != generation {
		uc.retireLocked()
	}
	if uc.current == nil {
		c, err := Dial(ctx, up)
		if err != nil {
			return nil, nil, err
		}
		uc.current = &cachedClient{conn: c, generation: generation}
	}
	c := uc.current
	c.active++
	var once sync.Once
	release := func() {
		once.Do(func() {
			uc.mu.Lock()
			defer uc.mu.Unlock()
			c.active--
			uc.closeLocked(c)
		})
	}
	return c.conn, release, nil
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
	if uc.current != nil && uc.current.conn == old {
		uc.retireLocked()
	}
}

func (s *Session) closeUpstreams() {
	s.mu.Lock()
	conns := make([]*upstreamConn, 0, len(s.conns))
	for _, uc := range s.conns {
		conns = append(conns, uc)
	}
	s.mu.Unlock()

	for _, uc := range conns {
		uc.mu.Lock()
		uc.retireLocked()
		uc.mu.Unlock()
	}
}

// Manager tracks live sessions. Upstream connections are always node-local; with a Store the
// session records are shared, so a node that has never seen a session id can still resume it
// (its upstream sessions are simply re-established on first use).
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	closed   bool
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
		id:                 hex.EncodeToString(b[:]),
		slug:               slug,
		version:            protocolVersion,
		clientCapabilities: json.RawMessage(`{}`),
		conns:              map[string]*upstreamConn{},
		last:               time.Now(),
	}
	m.mu.Lock()
	if m.closed || len(m.sessions) >= 10000 {
		m.mu.Unlock()
		return nil, errors.New("session capacity unavailable")
	}
	m.sessions[s.id] = s
	m.mu.Unlock()
	return s, nil
}

// Persist records a freshly created session in the shared store.
func (m *Manager) Persist(ctx context.Context, s *Session) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if m.Store == nil {
		return nil
	}
	s.mu.Lock()
	s.persisted = time.Now()
	s.mu.Unlock()
	return m.Store.Save(ctx, Record{Owner: s.owner, ID: s.id, Slug: s.slug, KeySlug: s.keySlug, Version: s.version, ClientCapabilities: s.clientCapabilities})
}

// Resolve checks the authoritative store on every request, including warm caches.
// There is no authorization lease: database failures never fall back to local sessions.
func (m *Manager) Resolve(ctx context.Context, id string) (*Session, error) {
	if id == "" {
		return nil, nil
	}
	if m.Store == nil {
		return m.Get(id), nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rec, ok, err := m.Store.Load(ctx, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		m.Delete(id)
		return nil, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing := m.sessions[id]; existing != nil {
		existing.mu.Lock()
		existing.initialized = rec.Initialized
		existing.mu.Unlock()
		return existing, nil
	}
	if m.closed || len(m.sessions) >= 10000 {
		return nil, errors.New("session capacity unavailable")
	}
	s := &Session{owner: rec.Owner, id: rec.ID, slug: rec.Slug, keySlug: rec.KeySlug, version: rec.Version, initialized: rec.Initialized, clientCapabilities: rec.ClientCapabilities, conns: map[string]*upstreamConn{}, last: time.Now(), persisted: rec.LastSeen}
	m.sessions[id] = s
	return s, nil
}

// Touch refreshes the shared liveness of a session at most once per touchEvery.
func (m *Manager) Touch(ctx context.Context, s *Session) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
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
func (m *Manager) End(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if m.Store != nil {
		if err := m.Store.Delete(ctx, id); err != nil {
			return err
		}
	}
	m.Delete(id)
	return nil
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
		s.ended.Store(true)
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

// Ready returns the lifecycle state refreshed by the authoritative Resolve for this request.
func (m *Manager) Ready(ctx context.Context, s *Session) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.initialized, nil
}

func (m *Manager) Initialize(ctx context.Context, s *Session) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	if m.Store != nil {
		if err := m.Store.MarkInitialized(ctx, s.id); err != nil {
			return err
		}
	}
	s.initialized = true
	return nil
}

// Reconcile retires idle and active clients whose connection configuration changed.
// In-flight leases may finish within the request deadline; new calls cannot reuse them.
func (m *Manager) Reconcile(snap *config.Snapshot) {
	m.mu.RLock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.mu.RUnlock()
	for _, s := range sessions {
		s.mu.Lock()
		conns := make(map[string]*upstreamConn, len(s.conns))
		for id, uc := range s.conns {
			conns[id] = uc
		}
		s.mu.Unlock()
		for id, uc := range conns {
			up, ok := snap.Upstreams[id]
			uc.mu.Lock()
			if uc.current != nil && (!ok || uc.current.generation != up.Fingerprint()) {
				uc.retireLocked()
			}
			uc.mu.Unlock()
		}
	}
}

// Close retires local upstream connections without deleting shared sessions.
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	sessions := m.sessions
	m.sessions = map[string]*Session{}
	m.mu.Unlock()
	for _, s := range sessions {
		s.closeUpstreams()
	}
}
