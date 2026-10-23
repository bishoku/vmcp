package controlplane_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/controlplane"
	"github.com/bishoku/vmcp/core/internal/mockupstream"
)

func mockPayload(slug string) map[string]any {
	return map[string]any{"slug": slug, "name": slug, "kind": "rest", "url": "https://ignored.example", "rest": map[string]any{"mock": map[string]any{"namespace": "user-supplied-is-replaced"}, "allowInsecureHttp": true, "operations": []any{map[string]any{"name": "sample", "method": "GET", "path": "/sample", "enabled": true, "mockRules": []any{map[string]any{"name": "ok", "priority": 1, "response": map[string]any{"status": 200, "jsonBody": map[string]any{"slug": slug}}}}}}}}
}
func TestMockUpstreamsDisabledWithoutProvider(t *testing.T) {
	a := setup(t)
	_, provider := a.expect(200, "GET", "/api/v1/mock-provider", nil)
	if provider["enabled"] != false {
		t.Fatal(provider)
	}
	a.expect(422, "POST", "/api/v1/upstreams", mockPayload("mock"))
	a.expect(201, "POST", "/api/v1/upstreams", map[string]any{"slug": "normal", "name": "Normal MCP", "url": a.fixture.URL + "/mcp", "headers": map[string]string{"Authorization": "Bearer up-secret"}})
}
func TestMockUpstreamsLifecycleAndOutageIsolation(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	var mu sync.Mutex
	mappings := map[string]map[string]any{"external": {"id": "external", "metadata": map[string]any{"arbitrary": "unrelated"}}}
	wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if strings.HasPrefix(r.URL.Path, "/__admin/") {
			if r.Method == "GET" {
				items := []any{}
				for _, v := range mappings {
					items = append(items, v)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"mappings": items})
				return
			}
			if r.Method == "DELETE" {
				id := strings.TrimPrefix(r.URL.Path, "/__admin/mappings/")
				if id == r.URL.Path {
					t.Errorf("global delete attempted: %s", r.URL.Path)
				}
				delete(mappings, id)
				w.WriteHeader(200)
				return
			}
			var m map[string]any
			if json.NewDecoder(r.Body).Decode(&m) != nil {
				w.WriteHeader(400)
				return
			}
			mappings[m["id"].(string)] = m
			w.WriteHeader(201)
			return
		}
		for _, m := range mappings {
			request, _ := m["request"].(map[string]any)
			if request["urlPath"] == r.URL.Path {
				response := m["response"].(map[string]any)
				_ = json.NewEncoder(w).Encode(response["jsonBody"])
				return
			}
		}
		w.WriteHeader(404)
	}))
	defer wire.Close()
	a := setup(t, func(s *controlplane.Server) { s.Store.WireMockURL = wire.URL })
	headers, first := a.expect(201, "POST", "/api/v1/upstreams", mockPayload("one"))
	_, second := a.expect(201, "POST", "/api/v1/upstreams", mockPayload("two"))
	if first["url"] != wire.URL {
		t.Fatal("provider URL not enforced", first)
	}
	ns1 := first["rest"].(map[string]any)["mock"].(map[string]any)["namespace"].(string)
	ns2 := second["rest"].(map[string]any)["mock"].(map[string]any)["namespace"].(string)
	if ns1 == ns2 || len(ns1) != 32 {
		t.Fatal("namespace isolation failed")
	}
	_, state := a.expect(200, "GET", "/api/v1/upstreams/one/mock/status", nil)
	if state["state"] != "pending" {
		t.Fatal(state)
	}
	a.expect(412, "POST", "/api/v1/upstreams/one/mock/sync", map[string]any{})
	code, _, state := a.do("POST", "/api/v1/upstreams/one/mock/sync", map[string]any{}, map[string]string{"If-Match": headers.Get("ETag")})
	if code != 200 || state["state"] != "active" {
		t.Fatalf("sync: %d %v", code, state)
	}
	_, state = a.expect(200, "GET", "/api/v1/upstreams/two/mock/status", nil)
	if state["state"] != "pending" {
		t.Fatal("scoped sync touched another mock", state)
	}
	twoHeaders, _ := a.expect(200, "GET", "/api/v1/upstreams/two", nil)
	code, _, state = a.do("POST", "/api/v1/upstreams/two/mock/sync", map[string]any{}, map[string]string{"If-Match": twoHeaders.Get("ETag")})
	if code != 200 || state["state"] != "active" {
		t.Fatal(state)
	}
	_, preview := a.expect(200, "POST", "/api/v1/upstreams/one/rest/preview", map[string]any{"operation": "sample", "arguments": map[string]any{}})
	if preview["url"] != wire.URL+mockupstream.Prefix(ns1)+"/sample" {
		t.Fatal("preview routing mismatch", preview)
	}
	code, _, result := a.do("POST", "/api/v1/upstreams/one/rest/test", map[string]any{"operation": "sample", "arguments": map[string]any{}}, map[string]string{"If-Match": headers.Get("ETag")})
	if code != 200 || result["isError"] == true {
		t.Fatal("test failed", result)
	}
	// A changed config is pending until reconciled; an unrelated mapping survives.
	rest := first["rest"].(map[string]any)
	ops := rest["operations"].([]any)
	ops[0].(map[string]any)["mockRules"].([]any)[0].(map[string]any)["response"].(map[string]any)["jsonBody"] = map[string]any{"changed": true}
	headers, _ = a.expect(200, "PATCH", "/api/v1/upstreams/one", map[string]any{"rest": rest})
	_, state = a.expect(200, "GET", "/api/v1/upstreams/one/mock/status", nil)
	if state["state"] != "pending" {
		t.Fatal(state)
	}
	_, doc := a.expect(200, "GET", "/api/v1/export", nil)
	a.expect(200, "POST", "/api/v1/apply?dryRun=true", doc)
	a.expect(200, "POST", "/api/v1/apply", doc)
	a.expect(204, "DELETE", "/api/v1/upstreams/one", nil)
	h, _ := a.expect(200, "GET", "/api/v1/upstreams/two", nil)
	code, _, state = a.do("POST", "/api/v1/upstreams/two/mock/sync", map[string]any{}, map[string]string{"If-Match": h.Get("ETag")})
	if code != 200 {
		t.Fatal(state)
	}
	// Reconciliation of deletions is eventual; run a bounded background cycle.
	restore := func() {
		t.Helper()
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); a.control.RunMockUpstreams(ctx) }()
		defer func() { cancel(); <-done }()
		deadline := time.Now().Add(6 * time.Second)
		for time.Now().Before(deadline) {
			mu.Lock()
			count := len(mappings)
			_, external := mappings["external"]
			mu.Unlock()
			if count == 2 && external {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("background reconciliation failed")
	}
	restore()
	// Simulate lost mappings after restart, preserving unrelated definitions.
	mu.Lock()
	for id := range mappings {
		if id != "external" {
			delete(mappings, id)
		}
	}
	mu.Unlock()
	restore()
	wire.Close()
	_, state = a.expect(200, "GET", "/api/v1/upstreams/two/mock/status", nil)
	if state["state"] != "unavailable" {
		t.Fatal(state)
	}
	// Existing mocks remain manageable if the operator disables integration.
	a.control.Store.WireMockURL = ""
	a.expect(200, "PATCH", "/api/v1/upstreams/two", map[string]any{"enabled": false})
	a.expect(422, "POST", "/api/v1/upstreams", mockPayload("three"))
	// Saving mock config is independent of WireMock availability.
	a.expect(200, "PATCH", "/api/v1/upstreams/two", map[string]any{"name": "Saved while WireMock is down"})
	a.expect(201, "POST", "/api/v1/upstreams", map[string]any{"slug": "real", "name": "Normal MCP", "url": a.fixture.URL + "/mcp", "headers": map[string]string{"Authorization": "Bearer up-secret"}})
	a.expect(200, "POST", "/api/v1/upstreams/real/refresh-tools", nil)
	a.expect(200, "GET", "/api/v1/upstreams/real/tools", nil)
}
