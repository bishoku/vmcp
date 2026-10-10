package virtual

import (
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/netpolicy"
)

// ingress validates the transport boundary before authentication or body reads.
func ingress(next http.HandlerFunc) http.HandlerFunc {
	origins, configErr := netpolicy.Origins()
	return func(w http.ResponseWriter, r *http.Request) {
		if configErr != nil {
			http.Error(w, "invalid ingress configuration", 503)
			return
		}
		if values, ok := r.Header["Origin"]; ok {
			if len(values) != 1 {
				http.Error(w, "origin denied", 403)
				return
			}
			origin, err := netpolicy.CanonicalOrigin(values[0])
			if err != nil || !origins[origin] {
				http.Error(w, "origin denied", 403)
				return
			}
		}
		versions := r.Header.Values("MCP-Protocol-Version")
		if len(versions) > 1 || (len(versions) == 1 && !mcp.SupportsVersion(versions[0])) {
			http.Error(w, "unsupported MCP-Protocol-Version", 400)
			return
		}
		if r.Method == http.MethodPost {
			media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || media != "application/json" || (params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8")) {
				http.Error(w, "Content-Type must be application/json", 415)
				return
			}
			if !accepts(r, "application/json") || !accepts(r, "text/event-stream") {
				http.Error(w, "Accept must include application/json and text/event-stream", 406)
				return
			}
		}
		if r.Method == http.MethodGet && !accepts(r, "text/event-stream") {
			http.Error(w, "Accept must include text/event-stream", 406)
			return
		}
		next(w, r)
	}
}
func accepts(r *http.Request, want string) bool {
	for _, value := range r.Header.Values("Accept") {
		for _, part := range strings.Split(value, ",") {
			media, params, err := mime.ParseMediaType(strings.TrimSpace(part))
			if err != nil || media != want {
				continue
			}
			if q, ok := params["q"]; ok {
				value, err := strconv.ParseFloat(q, 64)
				if err != nil || !(value > 0 && value <= 1) {
					continue
				}
			}
			return true
		}
	}
	return false
}
