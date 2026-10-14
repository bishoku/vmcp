// Package audit records who called which tool, without recording payloads.
package audit

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Event is one audited action. Arguments and results are intentionally absent.
type Event struct {
	EventID       string
	OperationID   string
	RequestID     string
	TraceID       string
	SchemaVersion int
	ActorType     string
	ActorID       string
	Issuer        string
	Action        string
	Target        string
	Revision      int64
	Stage         string
	Time          time.Time
	VirtualServer string
	AgentKey      string // key slug; empty on open servers or unknown keys
	ClientIP      string
	// Subject is the JWT "sub" of an OAuth-authenticated caller (AgentKey then holds "oauth:<binding>").
	Subject    string
	Method     string // e.g. tools/call, initialize
	Tool       string // exposed tool name
	Upstream   string
	Status     string // ok | tool_error | error | denied
	Reason     string
	DurationMS int
	Redactions int // PII values masked in this call
}

// Recorder records standalone facts; strict implementations acknowledge storage synchronously.
type Recorder interface{ Record(Event) }

// Nop discards events.
type Nop struct{}

func (Nop) Record(Event) {}

// Writer batches events into Postgres on a background goroutine. When the buffer is
// full, events are dropped (and counted) rather than slowing agents down.
type Writer struct {
	pool            *pgxpool.Pool
	log             *slog.Logger
	ch              chan Event
	dropped         atomic.Int64
	done            chan struct{}
	mu              sync.Mutex
	stopped         bool
	failed          atomic.Int64
	dead            atomic.Int64
	pending         atomic.Int64
	oldest          atomic.Int64
	write           func(context.Context, []Event) error
	shutdownTimeout time.Duration
}

const (
	bufferSize = 4096
	batchSize  = 200
	flushEvery = time.Second
)

// NewWriter starts the background flusher; stop it by cancelling ctx, then wait on Done.
func NewWriter(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) *Writer {
	w := &Writer{pool: pool, log: log, ch: make(chan Event, bufferSize), done: make(chan struct{})}
	go w.run(ctx)
	return w
}

// Done is closed after the final flush.
func (w *Writer) Done() <-chan struct{} { return w.done }

// Dropped returns how many events were discarded because the buffer was full.
func (w *Writer) Dropped() int64 { return w.dropped.Load() }

// Record queues an event.
func (w *Writer) Record(e Event) {
	var err error
	e, err = Prepare(e)
	if err != nil {
		w.dead.Add(1)
		w.dropped.Add(1)
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		w.dropped.Add(1)
		return
	}
	w.pending.Add(1)
	select {
	case w.ch <- e:
		w.oldest.CompareAndSwap(0, e.Time.UnixNano())
	default:
		w.pending.Add(-1)
		if n := w.dropped.Add(1); n == 1 || n%1000 == 0 {
			w.log.Warn("best-effort audit buffer full", "dropped_total", n)
		}
	}
}

func (w *Writer) Stats() Counters {
	s := Counters{Mode: "best_effort", Dropped: w.dropped.Load(), FailedWrites: w.failed.Load(), DeadLetters: w.dead.Load(), Pending: w.pending.Load()}
	if oldest := w.oldest.Load(); oldest != 0 {
		s.OldestPendingAgeSeconds = max(0, time.Since(time.Unix(0, oldest)).Seconds())
	}
	return s
}

func (w *Writer) run(ctx context.Context) {
	defer close(w.done)
	t := time.NewTicker(flushEvery)
	defer t.Stop()
	batch := make([]Event, 0, batchSize)
	markedPermanent := false
	write := w.write
	if write == nil {
		write = func(ctx context.Context, evs []Event) error { return insert(ctx, w.pool, evs) }
	}
	flush := func(fctx context.Context) error {
		if len(batch) == 0 {
			return nil
		}
		err := write(fctx, batch)
		if err != nil {
			w.failed.Add(1)
			return err
		}
		w.mu.Lock()
		w.pending.Add(-int64(len(batch)))
		batch = batch[:0]
		markedPermanent = false
		if w.pending.Load() == 0 {
			w.oldest.Store(0)
		}
		w.mu.Unlock()
		return nil
	}
	// Once a batch fails, stop draining the bounded queue and retain that exact batch.
	retry := func(retryCtx context.Context) error {
		delay := 25 * time.Millisecond
		logged := false
		for {
			fctx, cancel := context.WithTimeout(retryCtx, WriteTimeout)
			err := flush(fctx)
			cancel()
			if err == nil {
				return nil
			}
			if !logged {
				w.log.Error("best-effort audit write failed; retaining batch", "events", len(batch))
				logged = true
			}
			if !markedPermanent && !transient(err) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				var pe *pgconn.PgError
				if errors.As(err, &pe) && (strings.HasPrefix(pe.Code, "22") || strings.HasPrefix(pe.Code, "23")) {
					var kept []Event
					dropped := 0
					for _, e := range batch {
						sctx, scancel := context.WithTimeout(retryCtx, WriteTimeout)
						serr := write(sctx, []Event{e})
						scancel()
						if serr == nil {
							dropped++
							continue
						}
						var spe *pgconn.PgError
						if errors.As(serr, &spe) && (strings.HasPrefix(spe.Code, "22") || strings.HasPrefix(spe.Code, "23")) {
							w.dead.Add(1)
							w.dropped.Add(1)
							dropped++
							w.log.Error("best-effort audit dropping invalid event", "event_id", e.EventID, "code", spe.Code)
							continue
						}
						kept = append(kept, e)
					}
					if dropped > 0 {
						w.mu.Lock()
						w.pending.Add(-int64(dropped))
						batch = kept
						if w.pending.Load() == 0 {
							w.oldest.Store(0)
						}
						w.mu.Unlock()
					}
					if len(batch) == 0 {
						return nil
					}
				}
				w.dead.Add(int64(len(batch)))
				markedPermanent = true
			}
			select {
			case <-retryCtx.Done():
				return retryCtx.Err()
			case <-time.After(delay):
			}
			delay = min(delay*2, 250*time.Millisecond)
		}
	}
	shutdown := func() {
		w.mu.Lock()
		w.stopped = true
		w.mu.Unlock()
		budget := w.shutdownTimeout
		if budget <= 0 {
			budget = 10 * time.Second
		}
		final, cancel := context.WithTimeout(context.Background(), budget)
		defer cancel()
		for {
			for len(batch) < batchSize {
				select {
				case e := <-w.ch:
					batch = append(batch, e)
				default:
					goto flushFinal
				}
			}
		flushFinal:
			if len(batch) == 0 {
				return
			}
			if err := retry(final); err != nil {
				lost := w.pending.Swap(0)
				w.dropped.Add(lost)
				w.log.Error("best-effort audit shutdown incomplete", "unpersisted_events", lost)
				return
			}
		}
	}
	for {
		select {
		case <-ctx.Done():
			shutdown()
			return
		case e := <-w.ch:
			batch = append(batch, e)
			if len(batch) < batchSize {
				continue
			}
		case <-t.C:
		}
		if err := retry(ctx); err != nil {
			shutdown()
			return
		}
	}
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func insert(ctx context.Context, pool *pgxpool.Pool, evs []Event) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `SET LOCAL synchronous_commit=on`); err != nil {
		return err
	}
	for _, e := range evs {
		if err := Insert(ctx, tx, e); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Purge deletes events older than the retention period and returns how many.
func Purge(ctx context.Context, pool *pgxpool.Pool, retention time.Duration) (int64, error) {
	if retention <= 0 {
		return 0, nil
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tag, err := pool.Exec(bounded, `WITH candidates AS (
 SELECT a.id FROM audit_events a WHERE a.ts < now() - make_interval(secs => $1)
 AND NOT EXISTS(SELECT 1 FROM audit_legal_holds h JOIN audit_events held ON held.event_id=h.event_id WHERE held.event_id=a.event_id OR (a.operation_id IS NOT NULL AND held.operation_id=a.operation_id))
 AND NOT EXISTS(SELECT 1 FROM audit_export_state consumer WHERE consumer.active AND EXISTS(
  SELECT 1 FROM audit_events sibling WHERE (sibling.event_id=a.event_id OR (a.operation_id IS NOT NULL AND sibling.operation_id=a.operation_id))
  AND NOT EXISTS(SELECT 1 FROM audit_deliveries d WHERE d.consumer=consumer.consumer AND d.event_id=sibling.event_id AND (d.state='delivered' OR (d.state='dead' AND sibling.ts < now() - make_interval(secs => $1 * 2))))))
 AND (a.operation_id IS NULL OR NOT EXISTS(SELECT 1 FROM audit_events p WHERE p.operation_id=a.operation_id AND p.stage='admission')
 OR (EXISTS(SELECT 1 FROM audit_events o WHERE o.operation_id=a.operation_id AND o.stage='outcome')
 AND NOT EXISTS(SELECT 1 FROM audit_events r WHERE r.operation_id=a.operation_id AND r.ts >= now() - make_interval(secs => $1))))
 ORDER BY a.ts,a.id LIMIT 500 FOR UPDATE OF a SKIP LOCKED)
 DELETE FROM audit_events a USING candidates c WHERE a.id=c.id
 AND NOT EXISTS(SELECT 1 FROM audit_events sibling WHERE a.operation_id IS NOT NULL AND sibling.operation_id=a.operation_id AND NOT EXISTS(SELECT 1 FROM candidates selected WHERE selected.id=sibling.id))`, retention.Seconds())
	return tag.RowsAffected(), err
}

