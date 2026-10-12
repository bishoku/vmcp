package auditexport

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

type discard struct{}

func (discard) Publish(context.Context, string, []byte) error { return nil }
func (discard) Close()                                        {}
func TestRestrictedExportRole(t *testing.T) {
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
	script, err := os.ReadFile("../../../deploy/postgres/audit-roles.sql")
	if err != nil {
		t.Fatal(err)
	}
	// PostgreSQL roles are cluster-wide; isolate names per database and remove on cleanup.
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	names := []string{"vmcp_audit_query", "vmcp_audit_export", "vmcp_audit_retention"}
	sql := string(script)
	for _, name := range names {
		sql = strings.ReplaceAll(sql, name, name+suffix)
	}
	if _, err = pool.Exec(ctx, sql); err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, name := range names {
			if _, err = pool.Exec(ctx, "DROP OWNED BY "+name+suffix+"; DROP ROLE "+name+suffix); err != nil {
				t.Error(err)
			}
		}
	}()
	if err = audit.Persist(ctx, pool, audit.Event{Action: "restricted-export", Status: "ok", Time: time.Now()}); err != nil {
		t.Fatal(err)
	}
	cfg := pool.Config()
	cfg.ConnConfig.RuntimeParams["role"] = "vmcp_audit_export" + suffix
	// A session role applies to every connection in the exporter pool.
	cfg.MaxConns = 2
	cfg.MinConns = 0
	rp, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rp.Close()
	if err = store.CheckTenant(ctx, rp); err != nil {
		t.Fatal(err)
	}
	worker, err := audit.NewPublishingExporter(rp, "restricted", discard{})
	if err != nil {
		t.Fatal(err)
	}
	if err = worker.Register(ctx); err != nil {
		t.Fatal(err)
	}
	if more, err := worker.Step(ctx); err != nil || !more {
		t.Fatal("restricted export failed", more, err)
	}
	if _, err = rp.Exec(ctx, "DELETE FROM audit_events"); err == nil {
		t.Fatal("export identity can delete evidence")
	}
}
