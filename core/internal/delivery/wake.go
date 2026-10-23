package delivery

// Subscribe registers a bounded wakeup channel. PostgreSQL remains the queue;
// Same-process writers wake readers immediately; other replicas poll durable rows.
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

// NotifyLocal wakes any in-process subscribers for session after durable writes commit.
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
