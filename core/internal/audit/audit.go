// Package audit records who called which tool, without recording payloads.
package audit

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Event is one audited action. Arguments and results are intentionally absent.
type Event struct {
	Time          time.Time
	VirtualServer string
	AgentKey      string // key slug; empty on open servers or unknown keys
	ClientIP      string
	Method        string // e.g. tools/call, initialize
	Tool          string // exposed tool name
	Upstream      string
	Status        string // ok | tool_error | error | denied
	Reason        string
	DurationMS    int
	Redactions    int // PII values masked in this call
}

// Recorder accepts events without blocking the request path.
type Recorder interface{ Record(Event) }

// Nop discards events.
type Nop struct{}

func (Nop) Record(Event) {}

// Writer batches events into Postgres on a background goroutine. When the buffer is
// full, events are dropped (and counted) rather than slowing agents down.
type Writer struct {
	pool    *pgxpool.Pool
	log     *slog.Logger
	ch      chan Event
	dropped atomic.Int64
	done    chan struct{}
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
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	select {
	case w.ch <- e:
	default:
		if n := w.dropped.Add(1); n == 1 || n%1000 == 0 {
			w.log.Warn("audit buffer full, dropping events", "dropped_total", n)
		}
	}
}

func (w *Writer) run(ctx context.Context) {
	defer close(w.done)
	t := time.NewTicker(flushEvery)
	defer t.Stop()
	batch := make([]Event, 0, batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		// Own context: the final flush must still run after ctx is cancelled.
		fctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := insert(fctx, w.pool, batch); err != nil {
			w.log.Error("audit write failed", "events", len(batch), "err", err)
		}
		batch = batch[:0]
	}
	for {
		select {
		case e := <-w.ch:
			batch = append(batch, e)
			if len(batch) >= batchSize {
				flush()
			}
		case <-t.C:
			flush()
		case <-ctx.Done():
			for {
				select {
				case e := <-w.ch:
					batch = append(batch, e)
				default:
					flush()
					return
				}
			}
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
	rows := make([][]any, len(evs))
	for i, e := range evs {
		rows[i] = []any{e.Time, e.VirtualServer, nullable(e.AgentKey), nullable(e.ClientIP), e.Method,
			nullable(e.Tool), nullable(e.Upstream), e.Status, nullable(e.Reason), e.DurationMS, e.Redactions}
	}
	_, err := pool.CopyFrom(ctx, pgx.Identifier{"audit_events"},
		[]string{"ts", "virtual_server", "agent_key", "client_ip", "method", "tool", "upstream", "status", "reason", "duration_ms", "redactions"},
		pgx.CopyFromRows(rows))
	return err
}

// Purge deletes events older than the retention period and returns how many.
func Purge(ctx context.Context, pool *pgxpool.Pool, retention time.Duration) (int64, error) {
	tag, err := pool.Exec(ctx, `DELETE FROM audit_events WHERE ts < now() - make_interval(secs => $1)`, retention.Seconds())
	return tag.RowsAffected(), err
}
