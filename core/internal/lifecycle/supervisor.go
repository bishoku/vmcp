package lifecycle

import (
	"context"
	"errors"
	"log/slog"
	"runtime"
	"sync"
)

type supervisorKey struct{}

// Supervisor fails the process closed when a critical worker panics. It does not
// restart damaged state or claim pending operations completed successfully.
type Supervisor struct {
	once sync.Once
	done chan struct{}
	log  *slog.Logger
	stop func()
}

func NewSupervisor(log *slog.Logger, stop func()) *Supervisor {
	return &Supervisor{done: make(chan struct{}), log: log, stop: stop}
}
func (s *Supervisor) Done() <-chan struct{} { return s.done }
func (s *Supervisor) Err() error {
	select {
	case <-s.done:
		return errors.New("critical worker failed; restart required")
	default:
		return nil
	}
}
func WithSupervisor(ctx context.Context, s *Supervisor) context.Context {
	return context.WithValue(ctx, supervisorKey{}, s)
}

// Guard must be deferred directly by the goroutine being protected. Panic values
// and stack argument values may contain credentials and are never logged.
func Guard(ctx context.Context, worker string) {
	if value := recover(); value != nil {
		s, _ := ctx.Value(supervisorKey{}).(*Supervisor)
		if s == nil {
			panic(value)
		}
		s.once.Do(func() {
			if s.stop != nil {
				s.stop()
			}
			pcs := make([]uintptr, 32)
			n := runtime.Callers(3, pcs)
			frames := runtime.CallersFrames(pcs[:n])
			sites := []string{}
			for {
				frame, more := frames.Next()
				sites = append(sites, frame.Function)
				if !more {
					break
				}
			}
			if s.log != nil {
				s.log.Error("critical worker panic", "worker", worker, "functions", sites)
			}
			close(s.done)
		})
	}
}
