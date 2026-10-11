package virtual

import (
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/pii"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCatalogRoutingCannotEscapeCuratedNamespace(t *testing.T) {
	vs := config.VirtualServer{Catalog: []config.VirtualCatalogObject{{UpstreamID: "a", Kind: "resource", Original: "doc://same", Exposed: config.CatalogName("alpha", "resource", "doc://same", "")}, {UpstreamID: "b", Kind: "resource", Original: "doc://same", Exposed: config.CatalogName("beta", "resource", "doc://same", "")}, {UpstreamID: "a", Kind: "template", Original: "doc://items/{id}", Exposed: config.CatalogName("alpha", "template", "doc://items/{id}", "")}}}
	for _, o := range vs.Catalog[:2] {
		got, original, ok := catalogRoute(vs, "resource", o.Exposed, true)
		if !ok || got.UpstreamID != o.UpstreamID || original != "doc://same" {
			t.Fatal("namespace collision")
		}
	}
	for _, uri := range []string{"doc://same", "https://127.0.0.1/secret", "vmcp://beta/template/doc://items/1", "vmcp://alpha/template/doc://other/1", "vmcp://alpha/template/prefix-doc://items/1", "vmcp://alpha/template/doc://items/1/suffix"} {
		if _, _, ok := catalogRoute(vs, "resource", uri, true); ok {
			t.Fatalf("unselected resource accepted: %s", uri)
		}
	}
	if _, original, ok := catalogRoute(vs, "resource", "vmcp://alpha/template/doc://items/123", true); !ok || original != "doc://items/123" {
		t.Fatal("template expansion rejected")
	}
}
func TestCatalogArgumentsPrivacyAndPagination(t *testing.T) {
	var definition mcp.CatalogObject
	_ = json.Unmarshal([]byte(`{"name":"review","arguments":[{"name":"subject","required":true}]}`), &definition)
	object := config.VirtualCatalogObject{Kind: "prompt", Definition: definition}
	for _, raw := range []string{`{}`, `{"arguments":{"subject":1}}`, `{"arguments":{"other":"x"}}`} {
		var params map[string]json.RawMessage
		_ = json.Unmarshal([]byte(raw), &params)
		if validatePromptArguments(object, params) == nil {
			t.Fatalf("invalid arguments accepted: %s", raw)
		}
	}
	var params map[string]json.RawMessage
	_ = json.Unmarshal([]byte(`{"arguments":{"subject":"x"}}`), &params)
	if err := validatePromptArguments(object, params); err != nil {
		t.Fatal(err)
	}
	masker, err := pii.Compile(pii.Config{Enabled: true, Detectors: []string{"email"}})
	if err != nil {
		t.Fatal(err)
	}
	vs := config.VirtualServer{PII: masker}
	for _, raw := range []string{`{"contents":[{"blob":"c2VjcmV0"}]}`, `{"messages":[{"content":{"type":"image","data":"c2VjcmV0"}}]}`} {
		if _, err := inspectCatalogResult([]byte(raw), vs); err == nil {
			t.Fatal("binary content bypassed privacy")
		}
	}
	raw, err := inspectCatalogResult([]byte(`{"messages":[{"content":{"type":"text","text":"person@example.com"}}]}`), vs)
	if err != nil || strings.Contains(string(raw), "person@example.com") {
		t.Fatal("prompt PII leaked")
	}
	vs.Catalog = make([]config.VirtualCatalogObject, 201)
	for i := range vs.Catalog {
		vs.Catalog[i] = config.VirtualCatalogObject{Kind: "prompt", Definition: definition}
	}
	h := &Handler{}
	msg, _ := mcp.NewRequest(mcp.NewIntID(1), "prompts/list", map[string]string{})
	w := httptest.NewRecorder()
	h.listCatalog(w, msg, vs, 7, "prompt", "prompts")
	var result struct {
		Result struct {
			Prompts    []mcp.CatalogObject `json:"prompts"`
			NextCursor string              `json:"nextCursor"`
		} `json:"result"`
	}
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Result.Prompts) != 200 || result.Result.NextCursor == "" {
		t.Fatal("catalog pagination missing")
	}
	msg, _ = mcp.NewRequest(mcp.NewIntID(2), "prompts/list", map[string]string{"cursor": result.Result.NextCursor})
	w = httptest.NewRecorder()
	h.listCatalog(w, msg, vs, 8, "prompt", "prompts")
	reply, _ := mcp.Decode(w.Body.Bytes())
	if reply.Error == nil {
		t.Fatal("stale cursor accepted")
	}
}
