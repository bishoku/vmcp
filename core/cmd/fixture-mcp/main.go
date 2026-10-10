// Command fixture-mcp runs a small MCP server for local development and demos.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/bishoku/vmcp/core/internal/agentauth"
	"github.com/bishoku/vmcp/core/internal/fixture"
)

func main() {
	addr := flag.String("addr", ":9100", "listen address")
	token := flag.String("token", "", "require this bearer token (optional)")
	sse := flag.Bool("sse", false, "answer tools/call as text/event-stream")
	legacy := flag.Bool("legacy-sse", false, "speak the legacy HTTP+SSE transport (GET opens a stream)")
	jwtIssuer := flag.String("jwt-issuer", "", "require an OAuth access token (JWT) from this issuer instead of -token")
	jwtAudience := flag.String("jwt-audience", "", "audience the JWT must carry (with -jwt-issuer)")
	jwtDiscovery := flag.String("jwt-discovery-url", "", "where to fetch OIDC discovery if it differs from the issuer")
	flag.Parse()

	mux := http.NewServeMux()
	opts := fixture.Options{Token: *token, SSE: *sse}
	var guard func(http.Handler) http.Handler = func(h http.Handler) http.Handler { return h }
	if *jwtIssuer != "" {
		v, err := agentauth.NewVerifier(context.Background(), agentauth.Config{Issuer: *jwtIssuer, DiscoveryURL: *jwtDiscovery, Audience: *jwtAudience})
		if err != nil {
			log.Fatal(err)
		}
		guard = func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
				if !ok {
					http.Error(w, "bearer token required", http.StatusUnauthorized)
					return
				}
				id, err := v.Verify(r.Context(), tok)
				if err != nil {
					log.Printf("rejected token: %v", err)
					http.Error(w, "invalid token", http.StatusUnauthorized)
					return
				}
				log.Printf("accepted token for %s", id.Subject)
				next.ServeHTTP(w, r)
			})
		}
	}
	if *legacy {
		mux.Handle("/sse", guard(fixture.NewLegacySSE(opts)))
	} else {
		mux.Handle("/mcp", guard(fixture.New(opts)))
	}

	log.Printf("fixture MCP server on %s/mcp (auth=%t sse=%t)", *addr, *token != "", *sse)
	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
