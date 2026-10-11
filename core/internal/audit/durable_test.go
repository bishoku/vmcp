package audit

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestDurableAdmissionRetryAndIdentity(t *testing.T) {
	var attempts atomic.Int32
	d := &Durable{write: func(_ context.Context, e Event) error {
		if e.Stage != "admission" || e.Status != "started" || !ValidID(e.EventID) || !ValidID(e.OperationID) || e.RequestID != e.OperationID {
			t.Errorf("invalid admission: %+v", e)
		}
		if attempts.Add(1) < 3 {
			return &pgconn.PgError{Code: "40001"}
		}
		return nil
	}}
	e, err := d.Admit(context.Background(), Event{Method: "tools/call", Action: "tools/call"})
	if err != nil || attempts.Load() != 3 || d.Stats().FailedWrites != 2 {
		t.Fatalf("retry: %+v %v", d.Stats(), err)
	}
	var ids []string
	d.write = func(_ context.Context, out Event) error {
		ids = append(ids, out.EventID)
		if out.Stage != "outcome" || out.OperationID != e.OperationID || out.RequestID != e.RequestID {
			t.Error("outcome lost correlation")
		}
		return nil
	}
	e.Status = "ok"
	if err := d.Complete(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	if err := d.Complete(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	if ids[0] != ids[1] || ids[0] == e.EventID {
		t.Fatal("outcome identity is unstable or equals admission")
	}
}

func TestDurableAdmissionRejectsDatabaseAndDiskFailures(t *testing.T) {
	for name, writeErr := range map[string]error{"database": errors.New("connection lost"), "disk_full": &pgconn.PgError{Code: "53100"}, "invalid_row": &pgconn.PgError{Code: "23514"}} {
		t.Run(name, func(t *testing.T) {
			d := &Durable{write: func(context.Context, Event) error { return writeErr }}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
			defer cancel()
			if _, err := d.Admit(ctx, Event{Method: "tools/call"}); err == nil {
				t.Fatal("failed acknowledgement accepted")
			}
			if d.Stats().BlockedAdmissions != 1 || d.Stats().FailedWrites == 0 {
				t.Fatalf("missing failure counters: %+v", d.Stats())
			}
			if name != "database" && d.Stats().DeadLetters != 1 {
				t.Fatal("permanent rejection not exposed")
			}
		})
	}
}

func TestTraceAndMetadataExclusion(t *testing.T) {
	good := "00-1234567890abcdef1234567890abcdef-1234567890abcdef-01"
	if TraceID(good) != "1234567890abcdef1234567890abcdef" {
		t.Fatal("trace missing")
	}
	for _, invalid := range []string{"secret", good + "-payload", "00-00000000000000000000000000000000-1234567890abcdef-01", "00-1234567890abcdef1234567890abcdef-0000000000000000-01"} {
		if TraceID(invalid) != "" {
			t.Fatalf("invalid trace retained: %s", invalid)
		}
	}
	if _, err := Prepare(Event{EventID: "caller-secret"}); err == nil {
		t.Fatal("arbitrary event ID accepted")
	}
	if _, err := Prepare(Event{RequestID: "caller-secret"}); err == nil {
		t.Fatal("arbitrary request ID accepted")
	}
}

func testWriter(write func(context.Context, []Event) error) *Writer {
	return &Writer{ch: make(chan Event, bufferSize), done: make(chan struct{}), log: slog.New(slog.NewTextHandler(io.Discard, nil)), write: write}
}
func waitAudit(t *testing.T, condition func() bool) {
	t.Helper()
	end := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(end) {
			t.Fatal("audit condition did not converge")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestWriterRetainsFailedBatchAndStableIDs(t *testing.T) {
	var attempts atomic.Int32
	var mu sync.Mutex
	var first []string
	var saved []string
	w := testWriter(func(_ context.Context, events []Event) error {
		mu.Lock()
		defer mu.Unlock()
		ids := make([]string, len(events))
		for i, e := range events {
			ids[i] = e.EventID
		}
		if attempts.Add(1) == 1 {
			first = ids
			return &pgconn.PgError{Code: "57P01"}
		}
		if len(ids) != len(first) {
			t.Error("failed batch changed size")
		}
		for i := range ids {
			if ids[i] != first[i] {
				t.Error("retry changed event ID")
			}
		}
		saved = append(saved, ids...)
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	go w.run(ctx)
	for i := 0; i < batchSize; i++ {
		w.Record(Event{Method: "tools/call", Status: "ok"})
	}
	waitAudit(t, func() bool { return w.Stats().Pending == 0 })
	cancel()
	<-w.Done()
	mu.Lock()
	defer mu.Unlock()
	if len(saved) != batchSize || w.Dropped() != 0 || w.Stats().FailedWrites != 1 {
		t.Fatalf("batch disappeared: saved=%d stats=%+v", len(saved), w.Stats())
	}
	unique := map[string]bool{}
	for _, id := range saved {
		unique[id] = true
	}
	if len(unique) != batchSize {
		t.Fatal("nonunique stable event IDs")
	}
	w.Record(Event{Method: "late", Status: "ok"})
	if w.Dropped() != 1 {
		t.Fatal("post-shutdown event silently queued")
	}
}

func TestFullBufferIsExplicitBestEffortLoss(t *testing.T) {
	w := testWriter(nil)
	w.ch = make(chan Event, 2)
	for i := 0; i < 3; i++ {
		w.Record(Event{Method: "tools/call", Status: "ok"})
	}
	if w.Stats().Pending != 2 || w.Dropped() != 1 {
		t.Fatalf("full buffer: %+v", w.Stats())
	}
}

func TestBestEffortShutdownFailureExplicitlyAccountsForPending(t *testing.T) {
	w := testWriter(func(context.Context, []Event) error { return &pgconn.PgError{Code: "53100"} })
	w.shutdownTimeout = 60 * time.Millisecond
	for i := 0; i < 3; i++ {
		w.Record(Event{Method: "tools/call", Status: "ok"})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	go w.run(ctx)
	<-w.Done()
	if w.Stats().Pending != 0 || w.Dropped() != 3 || w.Stats().FailedWrites == 0 || w.Stats().DeadLetters != 3 {
		t.Fatalf("shutdown hid failed batch: %+v", w.Stats())
	}
}
