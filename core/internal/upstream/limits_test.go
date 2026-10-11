package upstream

import (
	"context"
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"strings"
	"testing"
)

func TestDiscoveryBudgets(t *testing.T) {
	for _, mode := range []string{"cycle", "pages", "tools"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			_, err := listTools(context.Background(), func(context.Context, string, any) (json.RawMessage, error) {
				calls++
				page := mcp.ListToolsResult{NextCursor: "same"}
				if mode == "pages" {
					page.NextCursor = strings.Repeat("x", calls)
				}
				if mode == "tools" {
					page.Tools = make([]mcp.Tool, maxDiscoveryTools+1)
				}
				b, _ := json.Marshal(page)
				return b, nil
			})
			if err == nil || calls > maxDiscoveryPages {
				t.Fatalf("unbounded discovery: calls=%d error=%v", calls, err)
			}
		})
	}
}

func TestResponseAndMultilineSSEBudgets(t *testing.T) {
	if _, err := readBounded(strings.NewReader(strings.Repeat("x", maxSSEEventBytes+1))); err == nil {
		t.Fatal("oversized JSON accepted")
	}
	c := NewClient("https://example.com", nil, nil)
	frame := strings.Repeat("data: "+strings.Repeat("x", 1024)+"\n", maxSSEEventBytes/1024+1)
	if _, err := c.readSSE(strings.NewReader(frame), mcp.NewIntID(1)); err == nil {
		t.Fatal("oversized multiline SSE accepted")
	}
}
