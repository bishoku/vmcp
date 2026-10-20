package store

import (
	"github.com/bishoku/vmcp/core/internal/config"
	"testing"
)

func TestGuidanceRejectsAmbiguousCatalogAndReferences(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   VirtualServerInput
	}{
		{"prompt identity collision", VirtualServerInput{Catalog: []CatalogSelection{{Upstream: "remote", Kind: "prompt", Name: "review", Alias: "vmcp_local__review"}}, Guidance: &config.Guidance{Prompts: []config.LocalPrompt{{Name: "review", Messages: []config.PromptMessage{{Role: "user", Text: "review"}}}}}}},
		{"unknown tool reference", VirtualServerInput{Guidance: &config.Guidance{Instructions: "{{tool:missing}}"}}},
		{"argument outside prompt", VirtualServerInput{Guidance: &config.Guidance{Instructions: "{{arg:topic}}"}}},
		{"duplicate prompt arguments", VirtualServerInput{Guidance: &config.Guidance{Prompts: []config.LocalPrompt{{Name: "review", Arguments: []config.PromptArgument{{Name: "topic"}, {Name: "topic"}}, Messages: []config.PromptMessage{{Role: "user", Text: "review"}}}}}}},
		{"unselected tool", VirtualServerInput{Guidance: &config.Guidance{ToolReferences: []config.GuidanceToolReference{{Key: "echo", Upstream: "remote", Tool: "echo"}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if len(validateGuidance(tc.in)) == 0 {
				t.Fatal("ambiguous/invalid guidance accepted")
			}
		})
	}
}
