package controlplane_test

import (
	"context"
	"encoding/json"
	"testing"
)

func TestCallerContextConfigurationLifecycle(t *testing.T) {
	a := setup(t)
	cc := map[string]any{"enabled": true, "metaKey": "bank/context", "fields": []any{map[string]any{"name": "token", "header": "Authorization", "bearer": true, "required": true}}}
	payload := map[string]any{"slug": "context-rest", "name": "Context REST", "kind": "rest", "url": "https://example.com", "callerContext": cc, "rest": map[string]any{"auth": map[string]string{"type": "none"}, "operations": []any{map[string]any{"name": "get", "method": "GET", "path": "/", "enabled": true}}}}
	_, out := a.expect(201, "POST", "/api/v1/upstreams", payload)
	if out["callerContext"].(map[string]any)["enabled"] != true {
		t.Fatal(out)
	}
	var stored []byte
	if err := a.pool.QueryRow(context.Background(), `SELECT caller_context FROM upstreams WHERE slug='context-rest'`).Scan(&stored); err != nil || !json.Valid(stored) {
		t.Fatal(err)
	}
	_, out = a.expect(200, "PATCH", "/api/v1/upstreams/context-rest", map[string]any{"description": "keep context"})
	if out["callerContext"].(map[string]any)["enabled"] != true {
		t.Fatal("patch lost context")
	}
	a.expect(422, "PATCH", "/api/v1/upstreams/context-rest", map[string]any{"headers": map[string]string{"authorization": "static"}})
	_, doc := a.expect(200, "GET", "/api/v1/export", nil)
	a.expect(200, "POST", "/api/v1/apply?dryRun=true", doc)
	a.expect(200, "POST", "/api/v1/apply", doc)
	_, out = a.expect(200, "GET", "/api/v1/upstreams/context-rest", nil)
	if out["callerContext"].(map[string]any)["metaKey"] != "bank/context" {
		t.Fatal("apply lost context")
	}
	_, out = a.expect(200, "PATCH", "/api/v1/upstreams/context-rest", map[string]any{"callerContext": map[string]any{}})
	if out["callerContext"].(map[string]any)["enabled"] != false {
		t.Fatal("clear failed")
	}
}
