package upstream

import (
	"context"
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCatalogDiscoveryRejectsMalformedAndRepeatingPages(t *testing.T) {
	for _, test := range []struct {
		name     string
		response string
	}{{"missing", "{}"}, {"null", `{"resources":null}`}, {"duplicate", `{"resources":[{"name":"a","uri":"doc://a"},{"name":"b","uri":"doc://a"}]}`}, {"identity", `{"resources":[{"name":"a"}]}`}, {"repeat", `{"resources":[],"nextCursor":"same"}`}} {
		t.Run(test.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var message mcp.Message
				_ = json.NewDecoder(r.Body).Decode(&message)
				out := mcp.Message{JSONRPC: mcp.JSONRPCVersion, ID: message.ID, Result: json.RawMessage(test.response)}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(out)
			}))
			defer srv.Close()
			if _, err := DiscoverCatalog(context.Background(), NewClient(srv.URL, nil, nil), mcp.ServerCapabilities{Resources: &mcp.ResourcesCapability{}}); err == nil {
				t.Fatal("invalid discovery accepted")
			}
		})
	}
	c := NewClient("http://127.0.0.1:1", nil, nil)
	catalog, err := DiscoverCatalog(context.Background(), c, mcp.ServerCapabilities{})
	if err != nil || len(catalog.Resources) != 0 || len(catalog.Prompts) != 0 {
		t.Fatal("unadvertised capabilities were queried")
	}
}
