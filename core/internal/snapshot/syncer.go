// Package snapshot keeps the data plane's in-memory configuration in sync with Postgres.
package snapshot

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/store"
)

const notifyChannel = "vmcp_config"

// Syncer serves the latest good snapshot and refreshes it when the revision changes.
// Failed refreshes preserve data, but authorization fails closed when confirmation expires.
type Syncer struct {
	Pool         *pgxpool.Pool
	Box          *secrets.Box
	Log          *slog.Logger
	PollInterval time.Duration // fallback in case a NOTIFY is missed; default 5s

	// Configure before Start; no background callback may change these fields.
	MaxAuthorizationAge time.Duration // default 30s; cannot disable freshness
	OnChange            func(*config.Snapshot)
	now                 func() time.Time
	revision            func(context.Context) (int64, error)
	load                func(context.Context) (*config.Snapshot, error)
	current             atomic.Pointer[state]
	wake                chan struct{}
}

const DefaultMaxAuthorizationAge = 30 * time.Second

var ErrAuthorizationStale = errors.New("authorization snapshot is stale or unavailable")

type state struct {
	snapshot         *config.Snapshot
	confirmedAt      time.Time
	observedRevision int64
	refreshError     string
}

// Status exposes authorization freshness without configuration or secret data.
type Status struct {
	Ready            bool          `json:"ready"`
	Revision         int64         `json:"revision"`
	ObservedRevision int64         `json:"observedRevision"`
	ConfirmedAt      time.Time     `json:"confirmedAt"`
	Age              time.Duration `json:"ageNs"`
	MaxAge           time.Duration `json:"maxAgeNs"`
	Reason           string        `json:"reason"`
}

func (s *Syncer) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}
func (s *Syncer) maxAge() time.Duration {
	if s.MaxAuthorizationAge > 0 {
		return s.MaxAuthorizationAge
	}
	return DefaultMaxAuthorizationAge
}

// Snapshot is for configuration inspection/background work, never authorization admission.
func (s *Syncer) Snapshot() *config.Snapshot {
	st := s.current.Load()
	if st == nil {
		return nil
	}
	return st.snapshot
}

// AuthorizedSnapshot checks the snapshot and its confirmation in a single atomic read.
func (s *Syncer) AuthorizedSnapshot() (*config.Snapshot, error) {
	st := s.current.Load()
	if st == nil || s.clock().Sub(st.confirmedAt) >= s.maxAge() {
		return nil, ErrAuthorizationStale
	}
	return st.snapshot, nil
}
func (s *Syncer) Ready() bool { _, err := s.AuthorizedSnapshot(); return err == nil }
func (s *Syncer) Status() Status {
	status := Status{MaxAge: s.maxAge(), Reason: "snapshot not loaded"}
	if st := s.current.Load(); st != nil {
		status.Revision = st.snapshot.Revision
		status.ObservedRevision = st.observedRevision
		status.ConfirmedAt = st.confirmedAt
		status.Age = s.clock().Sub(st.confirmedAt)
		status.Ready = status.Age < status.MaxAge
		status.Reason = "fresh"
		if st.refreshError != "" {
			status.Reason = "refresh failed; serving within freshness window"
		}
		if !status.Ready {
			status.Reason = "authorization snapshot confirmation expired"
		}
	}
	return status
}

// Start performs the initial load, then keeps refreshing until ctx is done.
// It returns an error only if the initial load fails.
func (s *Syncer) Start(ctx context.Context) error {
	if s.PollInterval <= 0 {
		s.PollInterval = 5 * time.Second
	}
	// Poll often enough to confirm even when the configured freshness window is short.
	s.PollInterval = min(s.PollInterval, max(time.Nanosecond, s.maxAge()/3))
	s.wake = make(chan struct{}, 1)

	if s.MaxAuthorizationAge < 0 {
		return errors.New("MaxAuthorizationAge must be positive")
	}
	started := s.clock()
	snap, err := store.LoadSnapshot(ctx, s.Pool, s.Box)
	if err != nil {
		return err
	}
	s.current.Store(&state{snapshot: snap, confirmedAt: started, observedRevision: snap.Revision})
	s.Log.Info("snapshot loaded", "revision", snap.Revision,
		"upstreams", len(snap.Upstreams), "virtual_servers", len(snap.VirtualServers))

	if s.OnChange != nil {
		s.OnChange(snap)
	}
	go s.listen(ctx)
	go s.loop(ctx)
	return nil
}

func (s *Syncer) loop(ctx context.Context) {
	t := time.NewTicker(s.PollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-s.wake:
		}
		s.refresh(ctx)
	}
}

func (s *Syncer) refresh(ctx context.Context) {
	// Bound pool acquisition/query/decryption work as well as authorization age.
	ctx, cancel := context.WithTimeout(ctx, min(s.maxAge(), 5*time.Second))
	defer cancel()
	started := s.clock()
	revision := s.revision
	if revision == nil {
		revision = func(ctx context.Context) (int64, error) { return store.Revision(ctx, s.Pool) }
	}
	rev, err := revision(ctx)
	cur := s.current.Load()
	fail := func(observed int64, err error) {
		if cur != nil {
			copy := *cur
			copy.observedRevision = observed
			copy.refreshError = err.Error()
			s.current.Store(&copy)
		}
		if ctx.Err() == nil {
			s.Log.Warn("snapshot refresh failed; confirmation not renewed", "err", err)
		}
	}
	if err != nil {
		if cur != nil {
			fail(cur.observedRevision, err)
		}
		return
	}
	if cur != nil && rev < cur.observedRevision {
		fail(cur.observedRevision, errors.New("configuration revision moved backwards"))
		return
	}
	if cur != nil && cur.snapshot.Revision == rev {
		s.current.Store(&state{snapshot: cur.snapshot, confirmedAt: started, observedRevision: rev})
		return
	}
	if cur != nil && rev < cur.snapshot.Revision {
		fail(rev, errors.New("configuration revision moved backwards"))
		return
	}
	load := s.load
	if load == nil {
		load = func(ctx context.Context) (*config.Snapshot, error) { return store.LoadSnapshot(ctx, s.Pool, s.Box) }
	}
	snap, err := load(ctx)
	if err != nil {
		fail(rev, err)
		return
	}
	if snap.Revision < rev {
		fail(rev, errors.New("loaded snapshot precedes observed revision"))
		return
	}
	s.current.Store(&state{snapshot: snap, confirmedAt: started, observedRevision: snap.Revision})
	if s.OnChange != nil {
		s.OnChange(snap)
	}
	s.Log.Info("snapshot updated", "revision", snap.Revision, "upstreams", len(snap.Upstreams), "virtual_servers", len(snap.VirtualServers))
}

// listen wakes the refresh loop on NOTIFY, reconnecting on failure.
func (s *Syncer) listen(ctx context.Context) {
	for ctx.Err() == nil {
		if err := s.listenOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			s.Log.Warn("config listener lost; falling back to polling", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func (s *Syncer) listenOnce(ctx context.Context) error {
	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "LISTEN "+notifyChannel); err != nil {
		return err
	}
	// Catch changes that happened while we were not listening.
	s.trigger()
	for {
		if _, err := conn.Conn().WaitForNotification(ctx); err != nil {
			return err
		}
		s.trigger()
	}
}

func (s *Syncer) trigger() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
