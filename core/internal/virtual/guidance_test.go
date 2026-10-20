package virtual

import (
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"strings"
	"testing"
)

func TestLocalPromptDoesNotReinterpretToolNames(t *testing.T) {
	object := config.VirtualCatalogObject{Definition: mcp.CatalogObject{"arguments": json.RawMessage(`[{"name":"topic"}]`)}, Local: &config.LocalContent{ToolReferences: map[string]string{"read": "remote__{{arg:topic}}"}, Messages: []config.PromptMessage{{Role: "user", Text: "Use {{tool:read}} for {{arg:topic}}"}}}}
	raw, err := localCatalogResult(object, "prompts/get", map[string]json.RawMessage{"arguments": json.RawMessage(`{"topic":"customer"}`)})
	if err != nil || !strings.Contains(string(raw), "Use remote__{{arg:topic}} for customer") {
		t.Fatalf("tool name reinterpreted as template: %s %v", raw, err)
	}
}
