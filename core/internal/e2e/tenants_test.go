package e2e

import (
	"github.com/bishoku/vmcp/core/internal/controlplane"
	"github.com/bishoku/vmcp/core/internal/store"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDedicatedTenantsRejectCredentialsSessionsAndConfiguration(t *testing.T) {
	a, b := setup(t), setup(t)
	for _, tenant := range []struct {
		e         *env
		id, token string
	}{{a, "tenant-a", "vmcp_tenant_a"}, {b, "tenant-b", "vmcp_tenant_b"}} {
		tenant.e.exec(t, `UPDATE tenant_runtime_identity SET tenant_id=$1`, tenant.id)
		tenant.e.exec(t, `UPDATE virtual_servers SET auth_required=true WHERE slug='agent'`)
		tenant.e.exec(t, `INSERT INTO agent_keys(tenant_id,slug,key_hash,key_prefix) SELECT id,'same-slug',$1,'vmcp_test' FROM tenants WHERE slug='default'`, store.HashKey(tenant.token))
		tenant.e.exec(t, `INSERT INTO agent_key_servers SELECT k.id,v.id FROM agent_keys k,virtual_servers v WHERE k.slug='same-slug' AND v.slug='agent'`)
	}
	nodeA, _, _ := consistencyNode(t, a, nil)
	nodeB, _, _ := consistencyNode(t, b, nil)
	response, body := consistencyRequest(t, nodeA, "POST", "", "vmcp_tenant_a", consistencyInit)
	if response.StatusCode != 200 {
		t.Fatalf("tenant A initialize: %d %s", response.StatusCode, body)
	}
	sid := response.Header.Get("Mcp-Session-Id")
	if response, _ := consistencyRequest(t, nodeB, "POST", "", "vmcp_tenant_a", consistencyInit); response.StatusCode != http.StatusUnauthorized {
		t.Fatal("cross-tenant credential accepted")
	}
	if response, _ := consistencyRequest(t, nodeB, "POST", sid, "vmcp_tenant_b", consistencyCall); response.StatusCode != 404 {
		t.Fatalf("cross-tenant session accepted: %d", response.StatusCode)
	}
	a.exec(t, `UPDATE upstreams SET name='tenant-a-private' WHERE slug='alpha'`)
	var name string
	if err := b.pool.QueryRow(b.ctx, `SELECT name FROM upstreams WHERE slug='alpha'`).Scan(&name); err != nil || name == "tenant-a-private" {
		t.Fatal("configuration crossed tenant boundary", err)
	}
	var count int
	if err := b.pool.QueryRow(b.ctx, `SELECT count(*) FROM sessions WHERE id=$1`, sid).Scan(&count); err != nil || count != 0 {
		t.Fatal("session persisted in other tenant", err)
	}
	a.exec(t, `INSERT INTO audit_events(virtual_server,method,status) VALUES('agent','tenant-test','ok')`)
	var tenantID string
	if err := a.pool.QueryRow(a.ctx, `SELECT tenant_id FROM audit_events WHERE method='tenant-test'`).Scan(&tenantID); err != nil || tenantID != "tenant-a" {
		t.Fatal("audit tenant context lost", err)
	}
	if err := b.pool.QueryRow(b.ctx, `SELECT count(*) FROM audit_events WHERE method='tenant-test'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("audit crossed tenant boundary", err)
	}
}

func TestDedicatedTenantManagementAPICredentials(t *testing.T) {
	a, b := setup(t), setup(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	serverA := httptest.NewServer((&controlplane.Server{Store: &store.Store{Pool: a.pool, Box: a.syncer.Box}, AdminToken: "admin-a", Log: log}).Handler())
	defer serverA.Close()
	serverB := httptest.NewServer((&controlplane.Server{Store: &store.Store{Pool: b.pool, Box: b.syncer.Box}, AdminToken: "admin-b", Log: log}).Handler())
	defer serverB.Close()
	for _, tc := range []struct {
		url, token string
		want       int
	}{{serverA.URL, "admin-a", 200}, {serverB.URL, "admin-a", 401}, {serverB.URL, "admin-b", 200}} {
		request, _ := http.NewRequest("GET", tc.url+"/api/v1/upstreams", nil)
		request.Header.Set("Authorization", "Bearer "+tc.token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != tc.want {
			t.Fatalf("tenant API credential boundary: %d", response.StatusCode)
		}
	}
}
