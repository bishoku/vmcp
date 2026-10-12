package store

import "testing"

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
