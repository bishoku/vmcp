package store

import (
	"strings"
	"testing"
)

func TestUpstreamTransportHTTPOnly(t *testing.T) {
	for _, transport := range []string{"streamable_http", "sse", "process", "file", ""} {
		t.Run(transport, func(t *testing.T) {
			errs := ValidateUpstreamInput(UpstreamInput{Name: "HTTP server", URL: "https://example.com/mcp", Transport: transport})
			accepted := transport == "streamable_http" || transport == "sse"
			if accepted && len(errs) != 0 {
				t.Fatalf("HTTP transport rejected: %v", errs)
			}
			if !accepted {
				found := false
				for _, err := range errs {
					if err.Field == "/transport" {
						found = true
					}
				}
				if !found {
					t.Fatalf("unsupported transport accepted: %q", transport)
				}
			}
		})
	}
}

func TestCapabilityDescriptionUnicodeLimit(t *testing.T) {
	for _, text := range []string{"", "first line\n  indented\n", strings.Repeat("a", 20000), strings.Repeat("ş", 20000), strings.Repeat("🙂", 20000)} {
		in := VirtualServerInput{Name: "Descriptions", Tools: []ToolSelection{{Upstream: "alpha", Tool: "echo", Description: text}}}
		if errs := ValidateVirtualServerInput(in); len(errs) != 0 {
			t.Fatalf("valid description rejected: %v", errs)
		}
	}
	for _, character := range []string{"a", "ş", "🙂"} {
		errs := ValidateVirtualServerInput(VirtualServerInput{Name: "Descriptions", Tools: []ToolSelection{{Upstream: "alpha", Tool: "echo", Description: strings.Repeat(character, 20001)}}})
		if len(errs) != 1 || errs[0].Field != "/tools/0/description" {
			t.Fatalf("missing field-specific limit error: %v", errs)
		}
	}
}
