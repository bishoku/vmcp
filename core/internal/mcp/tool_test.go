package mcp

import (
	"encoding/json"
	"testing"
)

func TestToolMetadataAndExtensionRoundTrip(t *testing.T) {
	raw := []byte(`{"name":"original","title":"Display title","description":"upstream","inputSchema":{"type":"object"},"outputSchema":{"type":"object"},"_meta":{"revision":"1"},"x-vendor":{"field":true}}`)
	var tool Tool
	if err := json.Unmarshal(raw, &tool); err != nil {
		t.Fatal(err)
	}
	tool.Name = "alias"
	tool.Description = "approved"
	encoded, err := json.Marshal(tool)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(encoded, &fields)
	for _, key := range []string{"title", "outputSchema", "_meta", "x-vendor"} {
		if fields[key] == nil {
			t.Fatalf("tool field lost: %s", key)
		}
	}
	var name string
	_ = json.Unmarshal(fields["name"], &name)
	if name != "alias" {
		t.Fatal("extension replaced routing identity")
	}
	merged := MergeToolMetadata(Tool{Name: "route", Description: "override", InputSchema: json.RawMessage(`{}`)}, raw)
	if merged.Name != "route" || merged.Title != "Display title" || merged.Description != "override" {
		t.Fatal("cached tool metadata merge failed")
	}
}
