package omni

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bishoku/vmcp/core/internal/mcp"
)

func TestRemovedDiscoveryInputsAreRejected(t *testing.T) {
	h, _ := guidanceServer(t, false)
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"discover_server", map[string]any{"query": "orders"}},
		{"discover_tools", map[string]any{"query": "orders", "server_id": "sales"}},
		{"get_tool_schema", map[string]any{"tool_ref": "sales/orders", "server_id": "sales"}},
		{"invoke_tool", map[string]any{"tool_ref": "sales/orders", "arguments": map[string]any{}, "server_id": "sales"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := guidanceRPC(t, h, "Bearer valid", "tools/call", map[string]any{"name": tc.name, "arguments": tc.args})
			var response struct{ Error struct{ Code int } }
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Error.Code != mcp.CodeInvalidParams {
				t.Fatalf("removed API did not fail explicitly: %d %s", w.Code, w.Body)
			}
		})
	}
}

func TestDirectDiscoveryPreservesAuthorizationProjectAndRoutes(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8,::1/128")
	docs := []Document{
		{Ref: Ref("sales", "pg__orders"), Server: "sales", Name: "pg__orders", Project: "commerce"},
		{Ref: Ref("support", "pg__orders"), Server: "support", Name: "pg__orders", Project: "service"},
		{Ref: Ref("private", "pg__orders"), Server: "private", Name: "pg__orders", Project: "commerce"},
	}
	access := Access{Refs: map[string]string{}}
	index := NewIndex(nil)
	index.status.Ready = true
	for n, d := range docs {
		index.entries = append(index.entries, prepareEntry(d))
		if n < 2 {
			access.Refs[d.Ref] = d.Hash()
		}
	}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/omni/access" {
			t.Errorf("unexpected backend endpoint: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(access)
	}))
	defer backend.Close()
	b, err := NewBackend(backend.URL, strings.Repeat("s", 32))
	if err != nil {
		t.Fatal(err)
	}
	h := (&Server{Backend: b, Index: index}).Handler()
	for _, tc := range []struct {
		project string
		want    []string
	}{
		{"", []string{docs[0].Ref, docs[1].Ref}},
		{"commerce", []string{docs[0].Ref}},
		{"service", []string{docs[1].Ref}},
		{"missing", nil},
	} {
		t.Run("project="+tc.project, func(t *testing.T) {
			args := map[string]any{"query": "orders"}
			if tc.project != "" {
				args["project"] = tc.project
			}
			var result struct {
				StructuredContent struct {
					Tools []Hit `json:"tools"`
				} `json:"structuredContent"`
			}
			guidanceResult(t, guidanceRPC(t, h, "Bearer valid", "tools/call", map[string]any{"name": "discover_tools", "arguments": args}), &result)
			hits := result.StructuredContent.Tools
			if len(hits) != len(tc.want) {
				t.Fatalf("unexpected authorized routes: %+v", hits)
			}
			for n, ref := range tc.want {
				if hits[n].Ref != ref {
					t.Fatalf("route lost or substituted: %+v", hits)
				}
			}
		})
	}
}
