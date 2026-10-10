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
// If a refresh fails, the previous snapshot keeps being served.
type Syncer struct {
	Pool         *pgxpool.Pool
	Box          *secrets.Box
	Log          *slog.Logger
	PollInterval time.Duration // fallback in case a NOTIFY is missed; default 5s

	current atomic.Pointer[config.Snapshot]
	wake    chan struct{}
}

// Snapshot returns the current snapshot (never nil after Start succeeds).
func (s *Syncer) Snapshot() *config.Snapshot { return s.current.Load() }

// Ready reports whether a snapshot has been loaded.
func (s *Syncer) Ready() bool { return s.current.Load() != nil }

// Start performs the initial load, then keeps refreshing until ctx is done.
// It returns an error only if the initial load fails.
func (s *Syncer) Start(ctx context.Context) error {
	if s.PollInterval <= 0 {
		s.PollInterval = 5 * time.Second
	}
	s.wake = make(chan struct{}, 1)

	snap, err := store.LoadSnapshot(ctx, s.Pool, s.Box)
	if err != nil {
		return err
	}
	s.current.Store(snap)
	s.Log.Info("snapshot loaded", "revision", snap.Revision,
		"upstreams", len(snap.Upstreams), "virtual_servers", len(snap.VirtualServers))

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
	rev, err := store.Revision(ctx, s.Pool)
	if err != nil {
		if ctx.Err() == nil {
			s.Log.Warn("revision check failed; serving last snapshot", "err", err)
		}
		return
	}
	if cur := s.current.Load(); cur != nil && cur.Revision == rev {
		return
	}
	snap, err := store.LoadSnapshot(ctx, s.Pool, s.Box)
	if err != nil {
		if ctx.Err() == nil {
			s.Log.Error("snapshot reload failed; serving last snapshot", "err", err)
		}
		return
	}
	s.current.Store(snap)
	s.Log.Info("snapshot updated", "revision", snap.Revision,
		"upstreams", len(snap.Upstreams), "virtual_servers", len(snap.VirtualServers))
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
