package audit_test

import (
	"context"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestExporterRetryRestartRetentionAndLegalHold(t *testing.T) {
	ctx := context.Background()
	url := testdb.New(t)
	if err := store.Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := store.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var fail atomic.Bool
	fail.Store(true)
	ids := map[string]int{}
	sink := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("Idempotency-Key")
		if id == "" {
			t.Error("no stable event ID")
		}
		ids[id]++
		io.Copy(io.Discard, r.Body)
		if fail.Load() {
			w.WriteHeader(503)
		} else {
			w.WriteHeader(204)
		}
	}))
	defer sink.Close()
	exporter, err := audit.NewExporter(pool, "siem", sink.URL)
	if err != nil {
		t.Fatal(err)
	}
	exporter.Client = sink.Client()
	if err := exporter.Register(ctx); err != nil {
		t.Fatal(err)
	}
	if err := audit.Persist(ctx, pool, audit.Event{Action: "export-test", Status: "ok", Time: time.Now().Add(-72 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if n, err := audit.Purge(ctx, pool, 24*time.Hour); err != nil || n != 0 {
		t.Fatal("unexported event purged", n, err)
	}
	if more, err := exporter.Step(ctx); err != nil || !more {
		t.Fatal(err)
	}
	if n, err := audit.Purge(ctx, pool, 24*time.Hour); err != nil || n != 0 {
		t.Fatal("failed delivery purged", n, err)
	}
	fail.Store(false)
	if _, err := pool.Exec(ctx, `UPDATE audit_deliveries SET next_attempt=now()`); err != nil {
		t.Fatal(err)
	}
	restarted, _ := audit.NewExporter(pool, "siem", sink.URL)
	restarted.Client = sink.Client()
	if _, err := restarted.Step(ctx); err != nil {
		t.Fatal(err)
	}
	for _, count := range ids {
		if count != 2 {
			t.Fatal("retry did not retain stable ID")
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO audit_legal_holds(event_id,reason) SELECT event_id,'case-1' FROM audit_events`); err != nil {
		t.Fatal(err)
	}
	if n, err := audit.Purge(ctx, pool, 24*time.Hour); err != nil || n != 0 {
		t.Fatal("held event purged", n, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM audit_legal_holds`); err != nil {
		t.Fatal(err)
	}
	if n, err := audit.Purge(ctx, pool, 24*time.Hour); err != nil || n != 1 {
		t.Fatal("exported event not purged", n, err)
	}
}
func TestExporterFindsLateCommittedLowerIDs(t *testing.T) {
	ctx := context.Background()
	url := testdb.New(t)
	if err := store.Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := store.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	exporter, _ := audit.NewExporter(pool, "late", "https://example.com")
	if err := exporter.Register(ctx); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := audit.Insert(ctx, tx, audit.Event{Action: "late", Status: "ok"}); err != nil {
		t.Fatal(err)
	}
	if err := audit.Persist(ctx, pool, audit.Event{Action: "early", Status: "ok"}); err != nil {
		t.Fatal(err)
	}
	if err := exporter.Fill(ctx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := exporter.Fill(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_deliveries`).Scan(&n); err != nil || n != 2 {
		t.Fatal("late commit lost", n, err)
	}
}

func TestDeadLettersAndExpiredLeasesRemainRecoverable(t *testing.T) {
	ctx := context.Background()
	url := testdb.New(t)
	if err := store.Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := store.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	sink := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer sink.Close()
	exporter, _ := audit.NewExporter(pool, "dead", sink.URL)
	exporter.Client = sink.Client()
	if err := exporter.Register(ctx); err != nil {
		t.Fatal(err)
	}
	if err := audit.Persist(ctx, pool, audit.Event{Action: "dead", Status: "ok", Time: time.Now().Add(-36 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := exporter.Fill(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE audit_deliveries SET state='leased',lease_until=now()-interval '1 second',attempts=19`); err != nil {
		t.Fatal(err)
	}
	if more, err := exporter.Step(ctx); err != nil || !more {
		t.Fatal(err)
	}
	var state string
	if err := pool.QueryRow(ctx, `SELECT state FROM audit_deliveries`).Scan(&state); err != nil || state != "dead" {
		t.Fatal("failed dead-letter transition", state, err)
	}
	if n, err := audit.Purge(ctx, pool, 24*time.Hour); err != nil || n != 0 {
		t.Fatal("dead letter purged before 2x retention window", n, err)
	}
	if n, err := audit.Purge(ctx, pool, 12*time.Hour); err != nil || n != 1 {
		t.Fatal("dead letter blocked retention past 2x retention window", n, err)
	}
}
func TestRetentionBatchIsBounded(t *testing.T) {
	ctx := context.Background()
	url := testdb.New(t)
	if err := store.Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := store.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `INSERT INTO audit_events(ts,virtual_server,method,status) SELECT now()-interval '3 days','v','test','ok' FROM generate_series(1,510)`); err != nil {
		t.Fatal(err)
	}
	if n, err := audit.Purge(ctx, pool, 24*time.Hour); err != nil || n != 500 {
		t.Fatal("unbounded retention", n, err)
	}
	if n, err := audit.Purge(ctx, pool, 24*time.Hour); err != nil || n != 10 {
		t.Fatal("retention continuation", n, err)
	}
}

func TestRetentionIndexPlan(t *testing.T) {
	ctx := context.Background()
	url := testdb.New(t)
	if err := store.Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := store.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `INSERT INTO audit_events(ts,virtual_server,method,status) SELECT now()-(i%365)*interval '1 day','v','test','ok' FROM generate_series(1,20000) i`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ANALYZE audit_events`); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, `EXPLAIN (ANALYZE,BUFFERS) SELECT id FROM audit_events WHERE ts<now()-interval '90 days' ORDER BY ts,id LIMIT 500`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		t.Log(line)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

type uncertainPublisher struct {
	ids  []string
	fail bool
}

func (p *uncertainPublisher) Publish(_ context.Context, id string, _ []byte) error {
	p.ids = append(p.ids, id)
	if p.fail {
		return context.DeadlineExceeded
	}
	return nil
}
func (p *uncertainPublisher) Close() {}
func TestPublisherIndependentConsumersAndLostAcknowledgement(t *testing.T) {
	ctx := context.Background()
	url := testdb.New(t)
	if err := store.Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := store.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	a, b := &uncertainPublisher{fail: true}, &uncertainPublisher{}
	first, err := audit.NewPublishingExporter(pool, "kafka-evidence", a)
	if err != nil {
		t.Fatal(err)
	}
	second, err := audit.NewPublishingExporter(pool, "rabbit-evidence", b)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []*audit.Exporter{first, second} {
		if err = e.Register(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err = audit.Persist(ctx, pool, audit.Event{Action: "broker-retry", Status: "ok", Time: time.Now().Add(-72 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	for _, e := range []*audit.Exporter{first, second} {
		if _, err = e.Step(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := audit.Purge(ctx, pool, 24*time.Hour); err != nil || n != 0 {
		t.Fatal("one consumer's receipt released another's retention", n, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE audit_deliveries SET next_attempt=now()`); err != nil {
		t.Fatal(err)
	}
	a.fail = false
	// A new process uses the same consumer ledger, retrying the same event UUID.
	restarted, _ := audit.NewPublishingExporter(pool, "kafka-evidence", a)
	if _, err = restarted.Step(ctx); err != nil {
		t.Fatal(err)
	}
	if len(a.ids) != 2 || len(b.ids) != 1 || a.ids[0] != a.ids[1] || a.ids[0] != b.ids[0] {
		t.Fatal("stable IDs or independent consumers lost")
	}
	if n, err := audit.Purge(ctx, pool, 24*time.Hour); err != nil || n != 1 {
		t.Fatal("confirmed event not released", n, err)
	}
}
