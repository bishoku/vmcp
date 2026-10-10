// Command fixture-mcp runs a small MCP server for local development and demos.
package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/bishoku/vmcp/core/internal/fixture"
)

func main() {
	addr := flag.String("addr", ":9100", "listen address")
	token := flag.String("token", "", "require this bearer token (optional)")
	sse := flag.Bool("sse", false, "answer tools/call as text/event-stream")
	legacy := flag.Bool("legacy-sse", false, "speak the legacy HTTP+SSE transport (GET opens a stream)")
	flag.Parse()

	mux := http.NewServeMux()
	opts := fixture.Options{Token: *token, SSE: *sse}
	if *legacy {
		mux.Handle("/sse", fixture.NewLegacySSE(opts))
	} else {
		mux.Handle("/mcp", fixture.New(opts))
	}

	log.Printf("fixture MCP server on %s/mcp (auth=%t sse=%t)", *addr, *token != "", *sse)
	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
