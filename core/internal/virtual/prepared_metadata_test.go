package virtual

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/pii"
	"net/http/httptest"
	"testing"
)

func TestPreparedListResponsesPreservePaginationMaskingAndErrors(t *testing.T) {
	masker, err := pii.Compile(pii.Config{Enabled: true, Detectors: []string{"email"}})
	if err != nil {
		t.Fatal(err)
	}
	v := config.VirtualServer{PII: masker}
	for i := 0; i < 205; i++ {
		name := fmt.Sprintf("item-%d", i)
		v.Tools = append(v.Tools, config.VirtualTool{Tool: mcp.Tool{Name: name, Description: "agent@example.com", InputSchema: json.RawMessage(`{"type":"object"}`), Meta: json.RawMessage(`{"owner":"agent@example.com"}`)}})
		v.Catalog = append(v.Catalog, config.VirtualCatalogObject{Kind: "resource", Exposed: name, Original: name, Definition: mcp.CatalogObject{"uri": json.RawMessage(fmt.Sprintf("%q", name)), "description": json.RawMessage(`"agent@example.com"`), "_meta": json.RawMessage(`{"owner":"agent@example.com"}`)}})
	}
	h := &Handler{}
	for _, invalid := range []bool{false, true} {
		if invalid {
			v.Tools[204].Tool.InputSchema = json.RawMessage(`bad`)
			v.Catalog[204].Definition["_meta"] = json.RawMessage(`bad`)
		}
		prepared := v.WithPreparedMetadata()
		for _, kind := range []string{"tool", "resource", "template", "prompt"} {
			for _, cursor := range []string{"", base64.RawURLEncoding.EncodeToString([]byte(kind + ":9:200")), base64.RawURLEncoding.EncodeToString([]byte(kind + ":8:200")), "invalid"} {
				params, _ := json.Marshal(map[string]string{"cursor": cursor})
				msg := &mcp.Message{JSONRPC: mcp.JSONRPCVersion, ID: mcp.NewIntID(7), Params: params}
				before, after := httptest.NewRecorder(), httptest.NewRecorder()
				if kind == "tool" {
					h.listTools(before, msg, v, 9)
					h.listTools(after, msg, prepared, 9)
				} else {
					h.listCatalog(before, msg, v, 9, kind, "objects")
					h.listCatalog(after, msg, prepared, 9, kind, "objects")
				}
				if before.Code != after.Code || before.Body.String() != after.Body.String() {
					t.Fatalf("response changed kind=%s cursor=%s invalid=%t\nbefore=%s\nafter=%s", kind, cursor, invalid, before.Body, after.Body)
				}
			}
		}
	}
}
