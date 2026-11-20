package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type input struct {
	Value string `json:"value"`
}

func main() {
	profile, ready := os.Args[1], os.Args[2]
	versions := map[string][]string{"legacy": {"2025-06-18"}, "modern": {"2026-07-28"}, "dual": {"2026-07-28", "2025-06-18"}}
	s := mcp.NewServer(&mcp.Implementation{Name: "interop-go", Version: "1"}, &mcp.ServerOptions{SupportedProtocolVersions: versions[profile]})
	mcp.AddTool(s, &mcp.Tool{Name: "echo", Description: "Echo a supplied value"}, func(_ context.Context, _ *mcp.CallToolRequest, args input) (*mcp.CallToolResult, any, error) {
		f, err := os.OpenFile(os.Getenv("VMCP_EXECUTIONS"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return nil, nil, err
		}
		_, err = fmt.Fprintln(f, args.Value)
		closeErr := f.Close()
		if err != nil {
			return nil, nil, err
		}
		if closeErr != nil {
			return nil, nil, closeErr
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: args.Value}}}, nil, nil
	})
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, &mcp.StreamableHTTPOptions{Stateless: profile != "legacy", JSONResponse: true})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(ready, []byte(fmt.Sprintf("http://%s/mcp", listener.Addr())), 0600); err != nil {
		panic(err)
	}
	if err := http.Serve(listener, h); err != nil {
		panic(err)
	}
}
