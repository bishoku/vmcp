package audit

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/bishoku/vmcp/core/internal/testdb"
	"github.com/bishoku/vmcp/core/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func batchDatabase(t testing.TB) *pgxpool.Pool {
	t.Helper()
	url := testdb.New(t)
	ctx := context.Background()
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err = provider.Up(ctx); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
func batchEvents(t testing.TB, n int) []*persistReq {
	t.Helper()
	out := make([]*persistReq, n)
	for i := range out {
		e, err := Prepare(Event{Stage: "event", Status: "ok", Method: "batch-test", Revision: 1})
		if err != nil {
			t.Fatal(err)
		}
		out[i] = &persistReq{event: e, done: make(chan error, 1)}
	}
	return out
}
func TestBatchAtomicityIdempotencyAndFallback(t *testing.T) {
	pool := batchDatabase(t)
	ctx := context.Background()
	batch := batchEvents(t, 4)
	if err := persistBatch(ctx, pool, batch); err != nil {
		t.Fatal(err)
	}
	if err := persistBatch(ctx, pool, batch); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events`).Scan(&count); err != nil || count != 4 {
		t.Fatalf("duplicate batch: %d %v", count, err)
	}
	// A database constraint error after a valid insert must roll the entire batch back.
	bad := batchEvents(t, 2)
	// Use the lifecycle uniqueness constraint, not event-id conflict (which is idempotent).
	op, _ := NewID()
	for _, r := range bad {
		r.event.OperationID = op
		r.event.Stage = "admission"
		r.event.Status = "started"
	}
	if err := persistBatch(ctx, pool, bad); err == nil {
		t.Fatal("conflicting lifecycle batch accepted")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events`).Scan(&count); err != nil || count != 4 {
		t.Fatalf("partial batch persisted: %d %v", count, err)
	}
	// Existing fallback acknowledges valid peers independently of an invalid event.
	peers := batchEvents(t, 2)
	peers[1].event.Status = "invalid"
	d := NewDurable(pool, nil)
	d.pendingBatch = peers
	d.flushing = true
	d.flushPendingBatches()
	if err := <-peers[0].done; err != nil {
		t.Fatal(err)
	}
	if err := <-peers[1].done; err == nil {
		t.Fatal("invalid peer accepted")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events`).Scan(&count); err != nil || count != 5 {
		t.Fatalf("fallback: %d %v", count, err)
	}
}

// Sequential reference retains the pre-optimization transaction/commit semantics.
func sequentialBatch(ctx context.Context, pool *pgxpool.Pool, batch []*persistReq) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SET LOCAL synchronous_commit=on`); err != nil {
		return err
	}
	for _, r := range batch {
		if err = Insert(ctx, tx, r.event); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func BenchmarkAuditBatch(b *testing.B) {
	pool := batchDatabase(b)
	ctx := context.Background()
	for _, size := range []int{8, 64} {
		for _, pipelined := range []bool{false, true} {
			b.Run(fmt.Sprintf("events=%d/pipeline=%t", size, pipelined), func(b *testing.B) {
				b.ReportAllocs()
				b.ReportMetric(float64(size), "events/batch")
				for b.Loop() {
					b.StopTimer()
					batch := batchEvents(b, size)
					b.StartTimer()
					var err error
					if pipelined {
						err = persistBatch(ctx, pool, batch)
					} else {
						err = sequentialBatch(ctx, pool, batch)
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
