package auditexport

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
)

type blockingPublisher struct{ started, closed chan struct{} }

func (p *blockingPublisher) Publish(ctx context.Context, _ string, _ []byte) error {
	close(p.started)
	<-ctx.Done()
	return ctx.Err()
}
func (p *blockingPublisher) Close() { close(p.closed) }
func TestShutdownPreservesUnconfirmedReceipt(t *testing.T) {
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
	if err = audit.Persist(ctx, pool, audit.Event{Action: "shutdown", Status: "ok", Time: time.Now()}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VMCP_AUDIT_EXPORT_DATABASE_URL", url)
	t.Setenv("VMCP_AUDIT_EXPORT_CONSUMER", "shutdown")
	p := &blockingPublisher{make(chan struct{}), make(chan struct{})}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Run(runCtx, slog.New(slog.NewJSONHandler(io.Discard, nil)), func() (audit.Publisher, error) { return p, nil })
	}()
	select {
	case <-p.started:
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not publish")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("shutdown did not drain")
	}
	select {
	case <-p.closed:
	default:
		t.Fatal("publisher not closed")
	}
	var state string
	if err = pool.QueryRow(ctx, "SELECT state FROM audit_deliveries WHERE consumer='shutdown'").Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "pending" {
		t.Fatal("unconfirmed event was lost", state)
	}
}
