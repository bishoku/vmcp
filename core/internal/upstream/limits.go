package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"io"
)

const maxDiscoveryPages = 100
const maxDiscoveryTools = 10000

func readBounded(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxSSEEventBytes+1))
	if len(b) > maxSSEEventBytes {
		return nil, errors.New("upstream response exceeds limit")
	}
	return b, err
}

func listTools(ctx context.Context, call func(context.Context, string, any) (json.RawMessage, error)) ([]mcp.Tool, error) {
	var all []mcp.Tool
	cursor := ""
	seen := map[string]bool{}
	for pages := 0; pages < maxDiscoveryPages; pages++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		raw, err := call(ctx, mcp.MethodToolsList, mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		var page mcp.ListToolsResult
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, err
		}
		if len(all)+len(page.Tools) > maxDiscoveryTools {
			return nil, errors.New("upstream discovery tool limit exceeded")
		}
		all = append(all, page.Tools...)
		if page.NextCursor == "" {
			return all, nil
		}
		if len(page.NextCursor) > 4096 || seen[page.NextCursor] {
			return nil, errors.New("upstream discovery invalid or repeated cursor")
		}
		seen[page.NextCursor] = true
		cursor = page.NextCursor
	}
	return nil, errors.New("upstream discovery page limit exceeded")
}
