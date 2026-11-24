package controlplane_test

import "testing"

func TestLocalToolDraftAndPersistence(t *testing.T) {
	a := setup(t)
	tool := map[string]any{"name": "guide", "description": "Show the button guide", "response": "Button{{if .Args.examples}} example{{end}}", "parameters": []any{map[string]any{"name": "examples", "type": "boolean", "default": false}}}
	const preview = "/api/v1/local-tool-preview"
	_, before := a.expect(200, "GET", "/api/v1/info", nil)
	for _, sample := range []struct {
		args map[string]any
		want string
	}{{map[string]any{}, "Button"}, {map[string]any{"examples": true}, "Button example"}} {
		_, got := a.expect(200, "POST", preview, map[string]any{"tool": tool, "arguments": sample.args})
		if got["text"] != sample.want {
			t.Fatal(got)
		}
	}
	a.expect(422, "POST", preview, map[string]any{"tool": tool, "arguments": map[string]any{"examples": "true"}})
	_, after := a.expect(200, "GET", "/api/v1/info", nil)
	if before["configRevision"] != after["configRevision"] {
		t.Fatal("preview published draft")
	}
	_, got := a.expect(201, "POST", "/api/v1/upstreams", map[string]any{"slug": "ui-guide", "name": "UI guide", "kind": "local", "local": map[string]any{"tools": []any{tool}}})
	if got["toolCount"] != float64(1) || got["url"] != "" {
		t.Fatal(got)
	}
	a.expect(200, "PATCH", "/api/v1/upstreams/ui-guide", map[string]any{"local": map[string]any{"tools": []any{}}})
	_, got = a.expect(200, "GET", "/api/v1/upstreams/ui-guide", nil)
	if got["toolCount"] != float64(0) {
		t.Fatal(got)
	}
}
