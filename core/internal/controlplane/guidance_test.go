package controlplane_test

import (
	"context"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/store"
	"strings"
	"testing"
)

func TestGuidanceAPICompatibilityAndRoundTrip(t *testing.T) {
	a := setup(t)
	a.expect(201, "POST", "/api/v1/upstreams", a.upstream())
	a.expect(200, "POST", "/api/v1/upstreams/fx/refresh-tools", nil)
	db := &store.Store{Pool: a.pool}
	if err := db.Do(context.Background(), func(tx *store.Tx) error {
		return tx.ReplaceCatalog(context.Background(), "fx", mcp.Catalog{Instructions: "Use echo carefully"})
	}); err != nil {
		t.Fatal(err)
	}
	_, catalog := a.expect(200, "GET", "/api/v1/upstreams/fx/catalog", nil)
	if catalog["instructions"] != "Use echo carefully" {
		t.Fatal(catalog)
	}
	guidance := map[string]any{"instructions": "Use {{tool:echo}}", "toolReferences": []any{map[string]any{"key": "echo", "upstream": "fx", "tool": "echo"}}, "resources": []any{map[string]any{"name": "guide", "enabled": true, "mimeType": "text/markdown", "text": strings.Repeat("🙂", 20000)}}, "prompts": []any{map[string]any{"name": "workflow", "enabled": true, "arguments": []any{map[string]any{"name": "topic", "required": true}}, "messages": []any{map[string]any{"role": "user", "text": "{{tool:echo}} {{arg:topic}}"}}}}, "upstreamInstructions": []string{"fx"}}
	body := map[string]any{"name": "Agent", "slug": "guided", "tools": []any{map[string]any{"upstream": "fx", "tool": "echo", "alias": "say"}}, "guidance": guidance}
	h, created := a.expect(201, "POST", "/api/v1/virtual-servers", body)
	if created["hasGuidance"] != true {
		t.Fatal(created)
	}
	_, list := a.expect(200, "GET", "/api/v1/virtual-servers", nil)
	for _, entry := range list["items"].([]any) {
		if _, ok := entry.(map[string]any)["guidance"]; ok {
			t.Fatal("inventory leaks full guidance payload")
		}
	}
	code, ph, patched := a.do("PATCH", "/api/v1/virtual-servers/guided", map[string]any{"guidance": map[string]any{"instructions": "Updated {{tool:echo}}"}}, map[string]string{"If-Match": h.Get("ETag")})
	if code != 200 || len(patched["guidance"].(map[string]any)["resources"].([]any)) != 1 {
		t.Fatalf("partial patch lost resource: %d %v", code, patched)
	}
	code, _, _ = a.do("PATCH", "/api/v1/virtual-servers/guided", map[string]any{"name": "Stale"}, map[string]string{"If-Match": h.Get("ETag")})
	if code != 412 {
		t.Fatalf("stale patch=%d", code)
	}
	delete(body, "guidance")
	delete(body, "slug")
	code, _, saved := a.do("PUT", "/api/v1/virtual-servers/guided", body, map[string]string{"If-Match": ph.Get("ETag")})
	if code != 200 || saved["guidance"].(map[string]any)["instructions"] != "Updated {{tool:echo}}" {
		t.Fatalf("legacy PUT lost guidance: %d", code)
	}
	_, doc := a.expect(200, "GET", "/api/v1/export", nil)
	a.expect(200, "POST", "/api/v1/apply?dryRun=true", doc)
	a.expect(200, "POST", "/api/v1/apply", doc)
	_, after := a.expect(200, "GET", "/api/v1/virtual-servers/guided", nil)
	if toJSON(after["guidance"]) != toJSON(saved["guidance"]) {
		t.Fatal("export/apply changed guidance")
	}
	// Referenced tool removal and invalid content are atomic.
	for _, patch := range []map[string]any{{"tools": []any{}}, {"guidance": map[string]any{"instructions": strings.Repeat("a", 4001)}}, {"guidance": map[string]any{"resources": []any{map[string]any{"name": "x", "mimeType": "text/plain", "text": strings.Repeat("🙂", 20001)}}}}, {"guidance": map[string]any{"instructions": "{{tool:unknown}}"}}, {"guidance": map[string]any{"upstreamInstructions": []string{"missing"}}}} {
		a.expect(422, "PATCH", "/api/v1/virtual-servers/guided", patch)
	}
	_, after = a.expect(200, "GET", "/api/v1/virtual-servers/guided", nil)
	if after["version"] != saved["version"] {
		t.Fatalf("invalid/no-op operations changed version %v -> %v", saved["version"], after["version"])
	}
	a.expect(200, "PATCH", "/api/v1/virtual-servers/guided", map[string]any{"guidance": map[string]any{"resources": []any{}}})
	_, after = a.expect(200, "GET", "/api/v1/virtual-servers/guided", nil)
	if len(after["guidance"].(map[string]any)["resources"].([]any)) != 0 || after["guidance"].(map[string]any)["instructions"] == "" {
		t.Fatal("array clear changed unrelated member")
	}
}

func TestUpstreamInstructionOnlySelectionProtectsDeletion(t *testing.T) {
	a := setup(t)
	a.expect(201, "POST", "/api/v1/upstreams", a.upstream())
	a.expect(201, "POST", "/api/v1/virtual-servers", map[string]any{"slug": "guide", "name": "Guide", "guidance": map[string]any{"upstreamInstructions": []string{"fx"}}})
	a.expect(409, "DELETE", "/api/v1/upstreams/fx", nil)
	a.expect(204, "DELETE", "/api/v1/upstreams/fx?force=true", nil)
	// The orphaned source remains visible and does not prevent unrelated edits.
	a.expect(200, "PATCH", "/api/v1/virtual-servers/guide", map[string]any{"name": "Renamed"})
	_, out := a.expect(200, "GET", "/api/v1/virtual-servers/guide", nil)
	if toJSON(out["guidance"].(map[string]any)["upstreamInstructions"]) != `["fx"]` {
		t.Fatal(out)
	}
}
