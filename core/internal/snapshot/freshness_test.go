package snapshot

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/config"
)

func TestAuthorizationFreshness(t *testing.T) {
	start := time.Now()
	now := start
	rev := int64(1)
	var queryErr, loadErr error
	loads := 0
	s := &Syncer{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), MaxAuthorizationAge: 30 * time.Second,
		now:      func() time.Time { return now },
		revision: func(context.Context) (int64, error) { return rev, queryErr },
		load: func(context.Context) (*config.Snapshot, error) {
			loads++
			return &config.Snapshot{Revision: rev}, loadErr
		},
	}
	if s.Ready() {
		t.Fatal("empty syncer is ready")
	}
	original := &config.Snapshot{Revision: 1}
	s.current.Store(&state{snapshot: original, confirmedAt: start, observedRevision: 1})
	now = start.Add(20 * time.Second)
	s.refresh(context.Background())
	if !s.current.Load().confirmedAt.Equal(now) || loads != 0 {
		t.Fatal("unchanged revision did not renew without load")
	}
	confirmed := now
	rev = 2
	loadErr = errors.New("decryption failure secret must not appear in status")
	now = confirmed.Add(29 * time.Second)
	s.refresh(context.Background())
	if !s.Ready() || s.Snapshot() != original || s.Status().ObservedRevision != 2 {
		t.Fatal("failed load changed snapshot or observation")
	}
	if !s.current.Load().confirmedAt.Equal(confirmed) {
		t.Fatal("failed load renewed freshness")
	}
	now = confirmed.Add(30 * time.Second)
	if snap, err := s.AuthorizedSnapshot(); snap != nil || !errors.Is(err, ErrAuthorizationStale) {
		t.Fatal("stale authorization accepted")
	}
	// A rollback to the old revision after observing a newer one must not renew.
	rev = 1
	s.refresh(context.Background())
	if s.Ready() {
		t.Fatal("revision rollback renewed stale authorization")
	}
	rev = 2
	loadErr = nil
	s.refresh(context.Background())
	if !s.Ready() || s.Snapshot().Revision != 2 {
		t.Fatal("recovery failed")
	}
	confirmed = now
	queryErr = errors.New("database unavailable")
	now = now.Add(31 * time.Second)
	s.refresh(context.Background())
	if s.Ready() || !s.current.Load().confirmedAt.Equal(confirmed) {
		t.Fatal("DB outage preserved authorization")
	}
}

func TestSlowConfirmationDoesNotExtendWindow(t *testing.T) {
	start := time.Now()
	now := start
	s := &Syncer{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), MaxAuthorizationAge: time.Second, now: func() time.Time { return now }}
	s.current.Store(&state{snapshot: &config.Snapshot{Revision: 1}, confirmedAt: start, observedRevision: 1})
	s.revision = func(context.Context) (int64, error) { now = now.Add(2 * time.Second); return 1, nil }
	s.refresh(context.Background())
	if s.Ready() {
		t.Fatal("slow query granted a new window after completion")
	}
	if (&Syncer{}).maxAge() != 30*time.Second {
		t.Fatal("zero config disabled default bound")
	}
}
