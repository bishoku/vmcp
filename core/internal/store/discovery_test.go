package store

import (
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"strings"
	"testing"
)

func TestCatalogDeltaIgnoresOrderAndFormattingButDetectsMetadataChanges(t *testing.T) {
	old := []mcp.CatalogObject{{"name": json.RawMessage(`"x"`), "uri": json.RawMessage(`"doc://x"`), "meta": json.RawMessage(`{"a":1,"b":2}`)}}
	next := []mcp.CatalogObject{{"meta": json.RawMessage(`{ "b": 2, "a": 1 }`), "uri": json.RawMessage(`"doc://x"`), "name": json.RawMessage(`"x"`)}}
	if !catalogDelta(old, next, "uri").empty() {
		t.Fatal("formatting became a change")
	}
	next[0]["meta"] = json.RawMessage(`{"a":2}`)
	if d := catalogDelta(old, next, "uri"); len(d.Changed) != 1 || d.Changed[0] != "doc://x" {
		t.Fatalf("lost metadata change: %+v", d)
	}
	if !catalogDelta(nil, []mcp.CatalogObject{}, "uri").empty() {
		t.Fatal("nil and empty lists differ")
	}
	d := DiscoveryChanges{Tools: CatalogDelta{Added: []string{strings.Repeat("x", 9000)}}}
	if len(d.Reason()) > 8192 || !strings.Contains(d.Reason(), "added=1") {
		t.Fatal("unbounded change summary")
	}
}
