//go:build acceptance

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Runs after all traffic and revocation checks. This host-side profile isolates
// snapshot work; it is not a measurement of the pod's CPU or memory limits.
func acceptanceSnapshotDiagnostic(t *testing.T, ctx context.Context, pool *pgxpool.Pool, master, output string) {
	t.Helper()
	box, err := secrets.NewBox(master)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(output, "snapshot-host.cpu.pprof"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := pprof.StartCPUProfile(f); err != nil {
		t.Fatal(err)
	}
	defer pprof.StopCPUProfile()
	loadCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	started := time.Now()
	previous, err := store.LoadSnapshot(loadCtx, pool, box)
	cancel()
	t.Logf("snapshot diagnostic: host initial elapsed=%s error=%v", time.Since(started), err)
	if err != nil {
		return
	}
	loadCtx, cancel = context.WithTimeout(ctx, 5*time.Second)
	started = time.Now()
	_, err = store.LoadSnapshotWithPrevious(loadCtx, pool, box, previous)
	cancel()
	t.Logf("snapshot diagnostic: host refresh budget=5s elapsed=%s error=%v", time.Since(started), err)
}
