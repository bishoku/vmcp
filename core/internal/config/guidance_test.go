package config

import (
	"github.com/bishoku/vmcp/core/internal/mcp"
	"strings"
	"testing"
)

func TestGuidanceCompilationPreservesCatalogAndFollowsAliases(t *testing.T) {
	g := Guidance{Instructions: "First {{tool:read}}", ToolReferences: []GuidanceToolReference{{Key: "read", Upstream: "alpha", Tool: "echo"}}, Resources: []LocalResource{{Name: "guide", Enabled: true, MimeType: "text/markdown", Text: "Call {{tool:read}}"}, {Name: "disabled", Text: "hidden"}}, Prompts: []LocalPrompt{{Name: "review", Enabled: true, Messages: []PromptMessage{{Role: "user", Text: "{{tool:read}} for {{arg:subject}}"}}}}, UpstreamInstructions: []string{"alpha"}}
	base := VirtualServer{Guidance: g, Tools: []VirtualTool{{UpstreamID: "a", UpstreamName: "echo", Tool: mcp.Tool{Name: "alias"}}}, Catalog: []VirtualCatalogObject{{Kind: "template", Exposed: "existing"}}}
	ups := map[string]Upstream{"a": {Slug: "alpha", Instructions: "Remote guide"}}
	got := base.WithGuidance(ups).WithPreparedMetadata()
	if got.Instructions != "First alias" || len(got.Catalog) != 4 || got.Catalog[0].Exposed != "existing" || got.Catalog[1].Local.Text != "Call alias" || got.Catalog[2].Local.Messages[0].Text != "{{tool:read}} for {{arg:subject}}" || got.Catalog[2].Local.ToolReferences["read"] != "alias" {
		t.Fatalf("compiled guidance: %+v", got)
	}
	base.Tools = nil
	got = base.WithGuidance(ups)
	if got.Instructions != "" || len(got.Catalog) != 2 {
		t.Fatalf("unavailable tool references must fail closed: %+v", got)
	}
}
func TestGuidanceRenderingDoesNotInterpretArgumentValues(t *testing.T) {
	result, err := RenderGuidance("Subject {{arg:name}}", nil, map[string]string{"name": "{{tool:secret}}"}, false)
	if err != nil || result != "Subject {{tool:secret}}" {
		t.Fatalf("%q %v", result, err)
	}
	if _, err = RenderGuidance("{{arg:name}}", nil, map[string]string{"name": strings.Repeat("a", 100001)}, false); err == nil {
		t.Fatal("oversized rendered content accepted")
	}
}

func TestGuidanceBoundsExpansionAndPreservesUnicode(t *testing.T) {
	if _, err := RenderGuidance(strings.Repeat("{{arg:text}}", 1000), nil, map[string]string{"text": strings.Repeat("x", 65536)}, false); err == nil {
		t.Fatal("repeated large arguments exceeded the output budget")
	}
	text := strings.Repeat("🙂", 100000)
	got, err := RenderGuidance("{{arg:text}}", nil, map[string]string{"text": text}, false)
	if err != nil || got != text {
		t.Fatalf("Unicode boundary was not preserved: %v", err)
	}
}
