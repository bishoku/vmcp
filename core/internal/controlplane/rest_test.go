package controlplane_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRESTLifecycleSecretsAndExplicitExecution(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	a := setup(t)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		u, p, ok := r.BasicAuth()
		if !ok || u != "service" || p != "rest-secret" || r.Header.Get("X-Operation") != "operation-secret" {
			t.Error("stored credentials missing")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	payload := map[string]any{"slug": "rest", "name": "REST", "kind": "rest", "url": srv.URL, "rest": map[string]any{"auth": map[string]any{"type": "basic", "username": "service", "password": "rest-secret"}, "allowInsecureHttp": true, "operations": []any{map[string]any{"name": "lookup", "method": "GET", "path": "/items", "enabled": true, "headers": map[string]string{"X-Operation": "operation-secret"}}}}}
	h, out := a.expect(201, "POST", "/api/v1/upstreams", payload)
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "rest-secret") || strings.Contains(string(raw), "operation-secret") {
		t.Fatal("secret leaked")
	}
	if out["kind"] != "rest" || out["toolCount"] != float64(1) || calls.Load() != 0 {
		t.Fatalf("%v calls=%d", out, calls.Load())
	}
	var ciphertext []byte
	if err := a.pool.QueryRow(context.Background(), `SELECT encrypted_rest_config FROM upstreams WHERE slug='rest'`).Scan(&ciphertext); err != nil || len(ciphertext) == 0 || strings.Contains(string(ciphertext), "rest-secret") {
		t.Fatal("not encrypted", err)
	}
	request := map[string]any{"operation": "lookup", "arguments": map[string]any{}}
	a.expect(200, "POST", "/api/v1/upstreams/rest/rest/preview", request)
	a.expect(422, "POST", "/api/v1/upstreams/rest/refresh-tools", nil)
	a.expect(422, "POST", "/api/v1/upstreams/rest/connection-test", nil)
	a.expect(412, "POST", "/api/v1/upstreams/rest/rest/test", request)
	if calls.Load() != 0 {
		t.Fatal("implicit execution")
	}
	code, _, result := a.do("POST", "/api/v1/upstreams/rest/rest/test", request, map[string]string{"If-Match": h.Get("ETag")})
	if code != 200 || result["isError"] == true || calls.Load() != 1 {
		t.Fatalf("test: %d %v", code, result)
	}
	_, reader := a.expect(201, "POST", "/api/v1/admin-tokens", map[string]any{"slug": "rest-reader", "scope": "read"})
	for _, action := range []string{"preview", "test"} {
		if code, _, _ := a.do("POST", "/api/v1/upstreams/rest/rest/"+action, request, map[string]string{"Authorization": "Bearer " + reader["token"].(string), "If-Match": h.Get("ETag")}); code != 403 {
			t.Fatalf("reader allowed %s: %d", action, code)
		}
	}
	a.expect(422, "PATCH", "/api/v1/upstreams/rest", map[string]any{"kind": "mcp"})
	a.expect(422, "PATCH", "/api/v1/upstreams/rest", map[string]any{"headers": map[string]string{"x-vmcp-agent-id": "spoof"}})
	_, doc := a.expect(200, "GET", "/api/v1/export", nil)
	raw, _ = json.Marshal(doc)
	if strings.Contains(string(raw), "rest-secret") || strings.Contains(string(raw), "operation-secret") {
		t.Fatal("export leaked secrets")
	}
	a.expect(200, "POST", "/api/v1/apply?dryRun=true", doc)
	a.expect(200, "POST", "/api/v1/apply", doc)
	if calls.Load() != 1 {
		t.Fatal("apply executed REST")
	}
	h, _ = a.expect(200, "GET", "/api/v1/upstreams/rest", nil)
	code, _, result = a.do("POST", "/api/v1/upstreams/rest/rest/test", request, map[string]string{"If-Match": h.Get("ETag")})
	if code != 200 || result["isError"] == true || calls.Load() != 2 {
		t.Fatalf("roundtrip: %d %v", code, result)
	}
	a.expect(200, "PATCH", "/api/v1/upstreams/rest", map[string]any{"enabled": false})
	if code, _, _ := a.do("POST", "/api/v1/upstreams/rest/rest/test", request, map[string]string{"If-Match": h.Get("ETag")}); code != 403 {
		t.Fatalf("disabled execution: %d", code)
	}
}

func TestNestedResponseProjectionConfigurationRoundtrip(t *testing.T) {
	a := setup(t)
	paths := [][]string{{"order", "items", "productId"}, {"order", "items", "quantity"}}
	operation := map[string]any{"name": "lookup", "method": "GET", "path": "/orders", "enabled": true, "responseSchema": json.RawMessage(`{"type":"object","properties":{"order":{"type":"object","properties":{"items":{"type":"array","items":{"type":"object","properties":{"productId":{"type":"string"},"quantity":{"type":"integer"},"cost":{"type":"number"}}}}}}}}`), "responseFieldPaths": paths}
	payload := map[string]any{"slug": "nested-rest", "name": "Nested REST", "kind": "rest", "url": "https://example.com", "rest": map[string]any{"auth": map[string]any{"type": "none"}, "timeoutSeconds": 30, "operations": []any{operation}}}
	a.expect(201, "POST", "/api/v1/upstreams", payload)
	check := func() {
		t.Helper()
		_, out := a.expect(200, "GET", "/api/v1/upstreams/nested-rest", nil)
		raw, _ := json.Marshal(out)
		var decoded struct {
			REST struct {
				Operations []struct{ ResponseFieldPaths [][]string }
			}
		}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		got, _ := json.Marshal(decoded.REST.Operations[0].ResponseFieldPaths)
		want, _ := json.Marshal(paths)
		if string(got) != string(want) {
			t.Fatalf("projection changed: %s != %s", got, want)
		}
	}
	check()
	paths = [][]string{{"order", "items", "productId"}}
	operation["responseFieldPaths"] = paths
	a.expect(200, "PATCH", "/api/v1/upstreams/nested-rest", map[string]any{"rest": payload["rest"]})
	check()
	_, doc := a.expect(200, "GET", "/api/v1/export", nil)
	a.expect(200, "POST", "/api/v1/apply?dryRun=true", doc)
	a.expect(200, "POST", "/api/v1/apply", doc)
	check()
	operation["responseFieldPaths"] = [][]string{{"order", "missing"}}
	a.expect(422, "PATCH", "/api/v1/upstreams/nested-rest", map[string]any{"rest": payload["rest"]})
	check()
}
