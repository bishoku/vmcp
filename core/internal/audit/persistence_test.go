package audit_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
)

func TestAdmissionSurvivesProcessCrash(t *testing.T) {
	if url := os.Getenv("VMCP_AUDIT_CRASH_CHILD_URL"); url != "" {
		pool, err := store.Connect(context.Background(), url)
		if err != nil {
			os.Exit(2)
		}
		d := audit.NewDurable(pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
		if _, err := d.Admit(context.Background(), audit.Event{Method: "tools/call", Action: "crash-test", VirtualServer: "agent"}); err != nil {
			os.Exit(3)
		}
		// Simulate abrupt death: no deferred shutdown or outcome writer runs.
		os.Exit(23)
	}
	url := testdb.New(t)
	ctx := context.Background()
	if err := store.Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	child := exec.Command(os.Args[0], "-test.run=^TestAdmissionSurvivesProcessCrash$")
	child.Env = append(os.Environ(), "VMCP_AUDIT_CRASH_CHILD_URL="+url)
	if err := child.Run(); err == nil {
		t.Fatal("child did not crash")
	} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 23 {
		t.Fatalf("child failed before admission: %v", err)
	}
	pool, err := store.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	status, err := audit.Pending(ctx, pool)
	if err != nil || status.Incomplete != 1 {
		t.Fatalf("crashed admission disappeared: %+v %v", status, err)
	}
	var e audit.Event
	if err := pool.QueryRow(ctx, `SELECT event_id::text,operation_id::text,request_id::text,method,virtual_server FROM audit_events WHERE stage='admission'`).Scan(&e.EventID, &e.OperationID, &e.RequestID, &e.Method, &e.VirtualServer); err != nil {
		t.Fatal(err)
	}
	// Retention preserves incomplete records even when their timestamps are old.
	if _, err := pool.Exec(ctx, `UPDATE audit_events SET ts=now()-interval '365 days'`); err != nil {
		t.Fatal(err)
	}
	if n, err := audit.Purge(ctx, pool, 24*time.Hour); err != nil || n != 0 {
		t.Fatalf("retention erased incomplete admission: %d %v", n, err)
	}
	restarted := audit.NewDurable(pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	// A reconciler may record a known error; never synthesize successful tool execution.
	e.Status = "error"
	e.Reason = "operator reconciliation: execution outcome unknown"
	if err := restarted.Complete(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err := audit.NewDurable(pool, nil).Complete(ctx, e); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE stage='outcome'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate delivery: %d %v", count, err)
	}
	if status, err := audit.Pending(ctx, pool); err != nil || status.Incomplete != 0 {
		t.Fatalf("outcome not linked: %+v %v", status, err)
	}
	// A recent outcome keeps its older admission, then the pair becomes eligible together.
	if n, err := audit.Purge(ctx, pool, 24*time.Hour); err != nil || n != 0 {
		t.Fatalf("retention split operation: %d %v", n, err)
	}
	// Clock skew can make an outcome older than admission; retention still preserves the pair.
	if _, err := pool.Exec(ctx, `UPDATE audit_events SET ts=CASE WHEN stage='admission' THEN now() ELSE now()-interval '365 days' END`); err != nil {
		t.Fatal(err)
	}
	if n, err := audit.Purge(ctx, pool, 24*time.Hour); err != nil || n != 0 {
		t.Fatalf("clock skew split operation: %d %v", n, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE audit_events SET ts=now()-interval '365 days'`); err != nil {
		t.Fatal(err)
	}
	if n, err := audit.Purge(ctx, pool, 24*time.Hour); err != nil || n != 2 {
		t.Fatalf("completed operation retention: %d %v", n, err)
	}
}
