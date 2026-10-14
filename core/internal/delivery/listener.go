package delivery

import (
	"context"
	"github.com/jackc/pgx/v5"
	"time"
)

// Subscribe registers a bounded wakeup channel. PostgreSQL remains the queue;
// lost/coalesced NOTIFY messages cannot lose events because readers also poll.
func (s *Store) Subscribe(session string) (<-chan struct{}, func()) {
	id := UUID()
	wake := make(chan struct{}, 1)
	s.watchMu.Lock()
	if s.watchers == nil {
		s.watchers = map[string]map[string]chan struct{}{}
	}
	if s.watchers[session] == nil {
		s.watchers[session] = map[string]chan struct{}{}
	}
	s.watchers[session][id] = wake
	s.watchMu.Unlock()
	return wake, func() {
		s.watchMu.Lock()
		defer s.watchMu.Unlock()
		delete(s.watchers[session], id)
		if len(s.watchers[session]) == 0 {
			delete(s.watchers, session)
		}
	}
}

// NotifyLocal wakes any in-process subscribers for session immediately without waiting for LISTEN/NOTIFY.
func (s *Store) NotifyLocal(session string) {
	s.watchMu.Lock()
	for _, wake := range s.watchers[session] {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
	s.watchMu.Unlock()
}
func (s *Store) listen(ctx context.Context) {
	for ctx.Err() == nil {
		s.listenOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}
func (s *Store) listenOnce(ctx context.Context) {
	url := s.ListenerURL
	if url == "" {
		url = s.Pool.Config().ConnConfig.ConnString()
	}
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		return
	}
	cfg.ConnectTimeout = 5 * time.Second
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = conn.Close(closeCtx)
	}()
	if _, err := conn.Exec(ctx, "LISTEN vmcp_delivery"); err != nil {
		return
	}
	for {
		notification, err := conn.WaitForNotification(ctx)
		if err != nil {
			return
		}
		s.watchMu.Lock()
		for _, wake := range s.watchers[notification.Payload] {
			select {
			case wake <- struct{}{}:
			default:
			}
		}
		s.watchMu.Unlock()
	}
}
