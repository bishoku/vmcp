package controlplane_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/health"
)

func TestNetworkPolicyCoversAdministrativeAndHealthPaths(t *testing.T) {
	a := setup(t)
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "")
	var hits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer target.Close()
	for _, transport := range []string{"streamable_http", "sse"} {
		_, result := a.expect(200, "POST", "/api/v1/connection-tests", map[string]any{"url": target.URL, "transport": transport, "headers": map[string]string{"Authorization": "Bearer must-not-leak"}})
		if result["ok"] != false {
			t.Fatalf("connection test allowed forbidden target: %v", result)
		}
	}
	a.expect(201, "POST", "/api/v1/upstreams", map[string]any{"slug": "denied", "name": "Denied", "url": target.URL, "transport": "streamable_http"})
	a.expect(502, "POST", "/api/v1/upstreams/denied/refresh-tools", nil)
	doc := map[string]any{"apiVersion": "vmcp/v1", "upstreams": []any{map[string]any{"slug": "preflight", "name": "Preflight", "url": target.URL, "transport": "streamable_http", "enabled": true}}, "virtualServers": []any{}}
	a.expect(422, "POST", "/api/v1/apply?dryRun=true", doc)
	a.expect(404, "GET", "/api/v1/upstreams/preflight", nil)
	_, _, err := health.Probe(context.Background(), config.Upstream{URL: target.URL, Transport: "streamable_http", FallbackURLs: []string{target.URL + "/fallback"}})
	if err == nil {
		t.Fatal("health probe bypassed policy")
	}
	if hits.Load() != 0 {
		t.Fatalf("denied upstream received %d requests", hits.Load())
	}
}
