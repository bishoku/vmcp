package auditexport

import (
	"context"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsIncludeUnclaimedBacklog(t *testing.T) {
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
	if err := audit.Persist(ctx, pool, audit.Event{Action: "fixture", Status: "ok", Time: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	exporter, err := audit.NewPublishingExporter(pool, "metrics", discard{})
	if err != nil {
		t.Fatal(err)
	}
	if err = exporter.Register(ctx); err != nil {
		t.Fatal(err)
	}
	m := &exportMetrics{}
	m.collect(ctx, exporter)
	if s := m.current.Load(); !s.available || s.oldest < 3500 {
		t.Fatalf("unclaimed backlog lost: %+v", s)
	}
	w := httptest.NewRecorder()
	m.ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	if strings.Contains(w.Body.String(), "fixture") || !strings.Contains(w.Body.String(), "metrics_available 1") {
		t.Fatal(w.Body.String())
	}
	_, err = exporter.Step(ctx)
	if err != nil {
		t.Fatal(err)
	}
	m.collect(ctx, exporter)
	if s := m.current.Load(); !s.available || s.oldest != 0 || s.counts["delivered"] != 1 {
		t.Fatalf("delivered backlog: %+v", s)
	}
}
