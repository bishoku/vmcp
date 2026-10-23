package controlplane

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/fixture"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/telemetry"
	"github.com/bishoku/vmcp/core/internal/testdb"
)

func discoveryServer(t *testing.T, url string) *Server {
	t.Helper()
	ctx := context.Background()
	database := testdb.New(t)
	if err := store.Migrate(ctx, database); err != nil {
		t.Fatal(err)
	}
	pool, err := store.Connect(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	s := &Server{Store: &store.Store{Pool: pool}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Metrics: telemetry.New()}
	t.Cleanup(s.CloseAudit)
	if err = s.Store.Do(ctx, func(tx *store.Tx) error {
		_, _, err := tx.PutUpstream(ctx, "source", store.UpstreamInput{Name: "Source", URL: url, Transport: "streamable_http", Enabled: true}, nil, false)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// Even explicit system payload capture must not audit background network probes.
	if _, err = pool.Exec(ctx, `UPDATE audit_settings SET settings='{"mode":"strict","scopes":{"system":{"request":true,"response":true}}}'`); err != nil {
		t.Fatal(err)
	}
	return s
}
func auditCount(t *testing.T, s *Server) int {
	t.Helper()
	var n int
	if err := s.Store.Pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_events`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func queueDiscovery(t *testing.T, s *Server) {
	t.Helper()
	if _, err := s.Store.Pool.Exec(context.Background(), `INSERT INTO mcp_discovery_requests(upstream_id) SELECT id FROM upstreams WHERE slug='source' ON CONFLICT(upstream_id) DO UPDATE SET lease_owner=NULL,lease_until=NULL`); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoveryAuditsChangesOnlyAndNoProbePayloads(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	fixtureServer := httptest.NewServer(fixture.New(fixture.Options{}))
	defer fixtureServer.Close()
	s := discoveryServer(t, fixtureServer.URL)
	ctx := context.Background()
	queueDiscovery(t, s)
	s.discoverOne(ctx)
	if n := auditCount(t, s); n != 1 {
		t.Fatalf("initial discovery should record one change, got %d", n)
	}
	var action, stage, reason, state string
	if err := s.Store.Pool.QueryRow(ctx, `SELECT action,stage,reason,payload_state FROM audit_events`).Scan(&action, &stage, &reason, &state); err != nil || action != "upstream.catalog_changed" || stage != "mutation" || state != "disabled" || !strings.Contains(reason, "echo") {
		t.Fatalf("change: %s %s %s %s %v", action, stage, reason, state, err)
	}
	before, _ := store.Revision(ctx, s.Store.Pool)
	// Reject every system audit insert: unchanged polling must still work.
	if _, err := s.Store.Pool.Exec(ctx, `CREATE FUNCTION reject_system_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.scope='system' THEN RAISE EXCEPTION 'audit unavailable'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_system_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_system_audit()`); err != nil {
		t.Fatal(err)
	}
	queueDiscovery(t, s)
	s.discoverOne(ctx)
	after, _ := store.Revision(ctx, s.Store.Pool)
	var queued int
	_ = s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM mcp_discovery_requests`).Scan(&queued)
	if auditCount(t, s) != 1 || queued != 0 || before != after {
		t.Fatalf("idle refresh wrote audit/config or failed: queued=%d revision %d/%d", queued, before, after)
	}
	recorder := httptest.NewRecorder()
	s.Metrics.ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(recorder.Body.String(), `activity="discovery",outcome="unchanged"} 1`) {
		t.Fatal(recorder.Body.String())
	}
}

func TestDiscoveryStrictChangeAuditRollsBackAndKeepsRetry(t *testing.T) {
	s := discoveryServer(t, "https://example.com")
	ctx := context.Background()
	result := &probeResult{Tools: []mcp.Tool{{Name: "new-tool", InputSchema: json.RawMessage(`{"type":"object"}`)}}, Catalog: mcp.Catalog{Instructions: "not-in-audit"}}
	if _, err := s.Store.Pool.Exec(ctx, `CREATE FUNCTION reject_change_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit unavailable'; END $$; CREATE TRIGGER reject_change_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_change_audit()`); err != nil {
		t.Fatal(err)
	}
	revision, _ := store.Revision(ctx, s.Store.Pool)
	var changes store.DiscoveryChanges
	if err := s.commitDiscovery(ctx, revision, "source", result, &changes); err == nil {
		t.Fatal("change committed without strict audit")
	}
	var tools int
	_ = s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM upstream_tools`).Scan(&tools)
	after, _ := store.Revision(ctx, s.Store.Pool)
	if tools != 0 || after != revision {
		t.Fatalf("change escaped transaction: tools=%d revision=%d/%d", tools, revision, after)
	}
	if _, err := s.Store.Pool.Exec(ctx, `DROP TRIGGER reject_change_audit ON audit_events`); err != nil {
		t.Fatal(err)
	}
	if err := s.commitDiscovery(ctx, revision, "source", result, &changes); err != nil {
		t.Fatal(err)
	}
	if auditCount(t, s) != 1 {
		t.Fatal("retry must record exactly one change")
	}
	var reason string
	_ = s.Store.Pool.QueryRow(ctx, `SELECT reason FROM audit_events`).Scan(&reason)
	if strings.Contains(reason, "not-in-audit") {
		t.Fatal("instruction contents leaked into audit metadata")
	}
}
func TestFailedDiscoveryIsOperationalOnly(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer failed.Close()
	s := discoveryServer(t, failed.URL)
	queueDiscovery(t, s)
	s.discoverOne(context.Background())
	if auditCount(t, s) != 0 {
		t.Fatal("routine network failure must not write audit")
	}
	var retry time.Time
	if err := s.Store.Pool.QueryRow(context.Background(), `SELECT lease_until FROM mcp_discovery_requests`).Scan(&retry); err != nil || retry.Before(time.Now()) {
		t.Fatalf("missing retry: %v %v", retry, err)
	}
}

func TestDiscoveryRelaxChangeAuditIsQueuedAfterCommit(t *testing.T) {
	s := discoveryServer(t, "https://example.com")
	ctx := context.Background()
	if _, err := s.Store.Pool.Exec(ctx, `UPDATE audit_settings SET settings='{"mode":"relax"}'`); err != nil {
		t.Fatal(err)
	}
	result := &probeResult{Tools: []mcp.Tool{{Name: "queued-tool", InputSchema: json.RawMessage(`{"type":"object"}`)}}}
	revision, _ := store.Revision(ctx, s.Store.Pool)
	var changes store.DiscoveryChanges
	if err := s.commitDiscovery(ctx, revision, "source", result, &changes); err != nil {
		t.Fatal(err)
	}
	s.CloseAudit()
	if auditCount(t, s) != 1 {
		t.Fatal("queued change was not flushed")
	}
	var mode string
	_ = s.Store.Pool.QueryRow(ctx, `SELECT audit_mode FROM audit_events`).Scan(&mode)
	if mode != "relax" {
		t.Fatalf("policy ignored: %s", mode)
	}
}
