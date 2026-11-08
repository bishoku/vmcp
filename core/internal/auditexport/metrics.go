package auditexport

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/bishoku/vmcp/core/internal/audit"
)

type exportStats struct {
	counts    map[string]int64
	oldest    float64
	collected time.Time
	available bool
}
type exportMetrics struct{ current atomic.Pointer[exportStats] }

func (m *exportMetrics) collect(ctx context.Context, e *audit.Exporter) {
	s := &exportStats{counts: map[string]int64{}, collected: time.Now()}
	raw, err := e.Stats(ctx)
	if err == nil {
		err = json.Unmarshal(raw, &s.counts)
	}
	if err == nil {
		err = e.Pool.QueryRow(ctx, `SELECT COALESCE(EXTRACT(EPOCH FROM now()-(SELECT a.ts FROM audit_events a WHERE NOT EXISTS(SELECT 1 FROM audit_deliveries d WHERE d.consumer=$1 AND d.event_id=a.event_id AND d.state='delivered') ORDER BY a.ts,a.id LIMIT 1)),0)`, e.Consumer).Scan(&s.oldest)
	}
	s.available = err == nil
	m.current.Store(s)
}
func (m *exportMetrics) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	s := m.current.Load()
	if s == nil || !s.available {
		fmt.Fprintln(w, "vmcp_audit_export_metrics_available 0")
		return
	}
	fmt.Fprintln(w, "vmcp_audit_export_metrics_available 1")
	fmt.Fprintf(w, "vmcp_audit_export_oldest_pending_seconds %g\nvmcp_audit_export_metrics_age_seconds %g\n", s.oldest, time.Since(s.collected).Seconds())
	for _, state := range []string{"pending", "leased", "dead", "delivered"} {
		fmt.Fprintf(w, "vmcp_audit_export_deliveries{state=%q} %d\n", state, s.counts[state])
	}
}

// The optional listener belongs on a private monitoring network, never ingress.
func startMetrics(m *exportMetrics) (func(), error) {
	addr := os.Getenv("VMCP_AUDIT_EXPORT_METRICS_ADDR")
	if addr == "" {
		return func() {}, nil
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", m)
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	go func() { _ = server.Serve(listener) }()
	return func() { _ = server.Close() }, nil
}
