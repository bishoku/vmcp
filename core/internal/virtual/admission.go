package virtual

import "sync"

// admissionSlot counts a request and its detached worker as one reservation.
type admissionSlot struct {
	mu      sync.Mutex
	refs    int
	release func()
}

func newAdmissionSlot(release func()) *admissionSlot {
	return &admissionSlot{refs: 1, release: release}
}
func (s *admissionSlot) retain() func() {
	s.mu.Lock()
	s.refs++
	s.mu.Unlock()
	return s.releaseOnce()
}
func (s *admissionSlot) releaseOnce() func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.refs--
			if s.refs == 0 {
				s.release()
			}
		})
	}
}
