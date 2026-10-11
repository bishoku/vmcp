package delivery

import (
	"bytes"
	"context"
	"encoding/base64"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
	"testing"
	"time"
)

func TestDeliveryBackpressureReservesTerminalAndExpiresReplay(t *testing.T) {
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
	box, _ := secrets.NewBox(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	s := &Store{Pool: pool, Box: box}
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(id,virtual_server,key_slug,protocol_version) VALUES('s','v','','2025-06-18')`); err != nil {
		t.Fatal(err)
	}
	op, err := s.Begin(ctx, "s", "1", "v", []byte("principal"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Append(ctx, op, bytes.Repeat([]byte("x"), 2<<20), false); err != ErrCapacity {
		t.Fatalf("oversized progress accepted: %v", err)
	}
	for i := 0; i < 255; i++ {
		if err := s.Append(ctx, op, []byte(`{}`), false); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Append(ctx, op, []byte(`{}`), false); err != ErrCapacity {
		t.Fatalf("unbounded queue: %v", err)
	}
	if err := s.Append(ctx, op, []byte(`{"id":1,"result":{}}`), true); err != nil {
		t.Fatal(err)
	}
	var event string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM mcp_events WHERE stream_id=$1 LIMIT 1`, op.Stream).Scan(&event); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE mcp_streams SET expires_at=now()-interval '1 second' WHERE id=$1`, op.Stream); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Resume(ctx, "s", event); err == nil {
		t.Fatal("expired replay accepted")
	}
	if err := s.Purge(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM mcp_events WHERE stream_id=$1`, op.Stream).Scan(&count); err != nil || count != 0 {
		t.Fatalf("expired events retained: %d %v", count, err)
	}
}

func TestDurableRoutingFencingReplayAndIsolation(t *testing.T) {
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
	box, _ := secrets.NewBox(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	s := &Store{Pool: pool, Box: box}
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(id,virtual_server,key_slug,protocol_version) VALUES('a','v','','2025-06-18'),('b','v','','2025-06-18')`); err != nil {
		t.Fatal(err)
	}
	op, err := s.Begin(ctx, "a", "1", "v", []byte("principal"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Begin(ctx, "a", "1", "v", []byte("principal")); err == nil {
		t.Fatal("duplicate operation accepted")
	}
	if err := s.Append(ctx, op, []byte(`{"jsonrpc":"2.0","method":"notifications/progress","params":{"progress":1}}`), false); err != nil {
		t.Fatal(err)
	}
	owner := UUID()
	if err := s.Claim(ctx, "a", op.Stream, owner); err != nil {
		t.Fatal(err)
	}
	if err := s.Claim(ctx, "a", op.Stream, UUID()); err == nil {
		t.Fatal("duplicate live stream owner")
	}
	events, err := s.Events(ctx, "a", op.Stream, owner, 0)
	if err != nil || len(events) != 1 {
		t.Fatalf("events %v %v", events, err)
	}
	if _, _, err := s.Resume(ctx, "b", events[0].ID); err == nil {
		t.Fatal("cross-session replay")
	}
	stream, seq, err := s.Resume(ctx, "a", events[0].ID)
	if err != nil || stream != op.Stream || seq != events[0].Seq {
		t.Fatal("resume cursor lost scope")
	}
	pending := UUID()
	if err := s.Request(ctx, op, pending); err != nil {
		t.Fatal(err)
	}
	if err := s.Reply(ctx, "b", pending, []byte(`{"result":{}}`)); err == nil {
		t.Fatal("cross-session reply")
	}
	if err := s.Reply(ctx, "a", pending, []byte(`{"result":{}}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.Reply(ctx, "a", pending, []byte(`{"result":{}}`)); err == nil {
		t.Fatal("duplicate reply accepted")
	}
	if raw, err := s.WaitReply(ctx, op, pending); err != nil || len(raw) == 0 {
		t.Fatal("reply did not reach originating worker")
	}
	if err := s.Cancel(ctx, "a", "1"); err != nil {
		t.Fatal(err)
	}
	cancelled, err := s.Renew(ctx, op)
	if err != nil || !cancelled {
		t.Fatal("cross-node cancellation missing")
	}
	stale := op
	stale.Fence = UUID()
	if err := s.Append(ctx, stale, []byte(`{}`), true); err == nil {
		t.Fatal("stale worker delivered event")
	}
	if err := s.Append(ctx, op, []byte(`{"jsonrpc":"2.0","id":1,"result":{}}`), true); err != nil {
		t.Fatal(err)
	}
	replay, err := s.Events(ctx, "a", stream, owner, seq)
	if err != nil || len(replay) != 1 || !replay[0].Terminal {
		t.Fatal("terminal replay missing")
	}
	var stored []byte
	if err := pool.QueryRow(ctx, `SELECT payload FROM mcp_events ORDER BY seq LIMIT 1`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if string(stored) == string(events[0].Payload) {
		t.Fatal("replay payload stored unencrypted")
	}
}

func TestListChangePublicationDeduplicatesAndWakes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	url := testdb.New(t)
	if err := store.Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := store.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	box, _ := secrets.NewBox(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	s := &Store{Pool: pool, Box: box}
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(id,virtual_server,key_slug,protocol_version) VALUES('s','v','','2025-06-18')`); err != nil {
		t.Fatal(err)
	}
	stream, err := s.GetStream(ctx, "s")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.PublishListChanged(ctx, 10); err != nil {
			t.Fatal(err)
		}
	}
	owner := UUID()
	if err := s.Claim(ctx, "s", stream, owner); err != nil {
		t.Fatal(err)
	}
	events, err := s.Events(ctx, "s", stream, owner, 0)
	if err != nil || len(events) != 1 {
		t.Fatalf("duplicate list changes: %d %v", len(events), err)
	}
	wake, unsubscribe := s.Subscribe("s")
	defer unsubscribe()
	go s.listen(ctx)
	// Wait for LISTEN without relying on wakeups as the queue.
	for end := time.Now().Add(2 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		var n int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND query='LISTEN vmcp_delivery'`).Scan(&n)
		if n > 0 {
			break
		}
	}
	if err := s.PublishListChanged(ctx, 11); err != nil {
		t.Fatal(err)
	}
	select {
	case <-wake:
	case <-time.After(time.Second):
		t.Fatal("NOTIFY did not wake reader")
	}
}

func TestExpiredWorkerCannotResumeOrDuplicateExecution(t *testing.T) {
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
	box, _ := secrets.NewBox(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	s := &Store{Pool: pool, Box: box}
	pool.Exec(ctx, `INSERT INTO sessions(id,virtual_server,key_slug,protocol_version) VALUES('s','v','','2025-06-18')`)
	op, err := s.Begin(ctx, "s", "1", "v", []byte("principal"))
	if err != nil {
		t.Fatal(err)
	}
	pool.Exec(ctx, `UPDATE mcp_operations SET lease_until=now()-interval '1 second' WHERE id=$1`, op.ID)
	if _, err := s.Renew(ctx, op); err == nil {
		t.Fatal("expired worker renewed its fence")
	}
	if err := s.Append(ctx, op, []byte(`{}`), true); err == nil {
		t.Fatal("expired worker published")
	}
	if err := s.Purge(ctx); err != nil {
		t.Fatal(err)
	}
	var state string
	pool.QueryRow(ctx, `SELECT state FROM mcp_operations WHERE id=$1`, op.ID).Scan(&state)
	if state != "abandoned" {
		t.Fatalf("lost worker not explicitly abandoned: %s", state)
	}
}
