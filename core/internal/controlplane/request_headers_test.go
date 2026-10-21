package controlplane_test

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRequestHeaderMappingLifecycle(t *testing.T) {
	a := setup(t)
	rules := []any{map[string]any{"source": "X-Agent-Token", "target": "X-Customer-Token", "required": true}}
	payload := map[string]any{"slug": "header-rest", "name": "Headers", "kind": "rest", "url": "https://example.com", "requestHeaderMappings": rules, "rest": map[string]any{"auth": map[string]string{"type": "none"}, "operations": []any{map[string]any{"name": "get", "method": "GET", "path": "/", "enabled": true}}}}
	_, out := a.expect(201, "POST", "/api/v1/upstreams", payload)
	if len(out["requestHeaderMappings"].([]any)) != 1 {
		t.Fatal(out)
	}
	var raw []byte
	if err := a.pool.QueryRow(context.Background(), `SELECT request_header_mappings FROM upstreams WHERE slug='header-rest'`).Scan(&raw); err != nil || !json.Valid(raw) {
		t.Fatal(err)
	}
	_, out = a.expect(200, "PATCH", "/api/v1/upstreams/header-rest", map[string]any{"description": "keep"})
	if len(out["requestHeaderMappings"].([]any)) != 1 {
		t.Fatal("patch lost rules")
	}
	a.expect(422, "PATCH", "/api/v1/upstreams/header-rest", map[string]any{"headers": map[string]string{"x-customer-token": "conflict"}})
	a.expect(422, "PATCH", "/api/v1/upstreams/header-rest", map[string]any{"callerContext": map[string]any{"enabled": true, "metaKey": "bank/context", "fields": []any{map[string]any{"name": "token", "header": "X-Customer-Token"}}}})
	a.expect(422, "PATCH", "/api/v1/upstreams/header-rest", map[string]any{"requestHeaderMappings": []any{map[string]any{"source": "Authorization", "target": "X-Token"}}})
	_, doc := a.expect(200, "GET", "/api/v1/export", nil)
	a.expect(200, "POST", "/api/v1/apply", doc)
	_, out = a.expect(200, "GET", "/api/v1/upstreams/header-rest", nil)
	if len(out["requestHeaderMappings"].([]any)) != 1 {
		t.Fatal("apply lost rules")
	}
	_, out = a.expect(200, "POST", "/api/v1/upstreams/header-rest/rest/preview", map[string]any{"operation": "get", "arguments": map[string]any{}})
	if out["headers"].(map[string]any)["X-Customer-Token"] != "[request header: X-Agent-Token]" {
		t.Fatal(out)
	}
	h, _ := a.expect(200, "GET", "/api/v1/upstreams/header-rest", nil)
	status, _, _ := a.do("POST", "/api/v1/upstreams/header-rest/rest/test", map[string]any{"operation": "get", "arguments": map[string]any{}}, map[string]string{"If-Match": h.Get("ETag")})
	if status != 422 {
		t.Fatalf("required mapping allowed admin execution: %d", status)
	}
	a.expect(422, "PATCH", "/api/v1/upstreams/header-rest", map[string]any{"rest": map[string]any{"auth": map[string]string{"type": "none"}, "operations": []any{map[string]any{"name": "get", "method": "GET", "path": "/", "enabled": true, "headers": map[string]string{"x-customer-token": "conflict"}}}}})
	a.expect(200, "PATCH", "/api/v1/upstreams/header-rest", map[string]any{"requestHeaderMappings": []any{}})
	_, out = a.expect(200, "GET", "/api/v1/upstreams/header-rest", nil)
	if v, ok := out["requestHeaderMappings"]; ok && len(v.([]any)) != 0 {
		t.Fatal("clear failed")
	}
	_, emptyDoc := a.expect(200, "GET", "/api/v1/export", nil)
	a.expect(200, "PATCH", "/api/v1/upstreams/header-rest", map[string]any{"requestHeaderMappings": rules})
	a.expect(200, "POST", "/api/v1/apply", emptyDoc)
	_, out = a.expect(200, "GET", "/api/v1/upstreams/header-rest", nil)
	if v, ok := out["requestHeaderMappings"]; ok && len(v.([]any)) != 0 {
		t.Fatal("export/apply did not restore empty mappings")
	}
	a.expect(201, "POST", "/api/v1/upstreams", map[string]any{"slug": "header-mcp", "name": "MCP headers", "url": "https://example.com/mcp", "requestHeaderMappings": rules})
	_, out = a.expect(200, "GET", "/api/v1/upstreams/header-mcp", nil)
	if len(out["requestHeaderMappings"].([]any)) != 1 {
		t.Fatal("MCP configuration lost")
	}

}
