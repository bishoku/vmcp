// Package tenancy hosts tenants through dedicated gateway/database deployments.
// Authentication remains authoritative at each tenant's plane, not at the router.
package tenancy

import (
	"errors"
	"github.com/bishoku/vmcp/core/internal/netpolicy"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type Tenant struct {
	ID      string `json:"id"`
	Host    string `json:"host"`
	Control string `json:"control"`
	Data    string `json:"data"`
	UI      string `json:"ui"`
}
type Router struct {
	routes map[string]map[string]*httputil.ReverseProxy
}

func New(tenants []Tenant) (*Router, error) {
	if len(tenants) == 0 || len(tenants) > 128 {
		return nil, errors.New("tenant directory requires 1..128 tenants")
	}
	r := &Router{routes: map[string]map[string]*httputil.ReverseProxy{}}
	ids := map[string]bool{}
	targets := map[string]string{}
	client := netpolicy.Client(&http.Client{Timeout: 75 * time.Second})
	for _, t := range tenants {
		if !regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`).MatchString(t.ID) || ids[t.ID] || t.Host != strings.ToLower(t.Host) || strings.ContainsAny(t.Host, "/:@% ") || !strings.Contains(t.Host, ".") || r.routes[t.Host] != nil {
			return nil, errors.New("invalid or duplicate tenant identity/host")
		}
		ids[t.ID] = true
		r.routes[t.Host] = map[string]*httputil.ReverseProxy{}
		for kind, raw := range map[string]string{"control": t.Control, "data": t.Data, "ui": t.UI} {
			target, err := url.Parse(raw)
			if err != nil || target.Scheme != "https" || target.Host == "" || target.User != nil || target.RawQuery != "" || target.Fragment != "" || (target.Path != "" && target.Path != "/") {
				return nil, errors.New("tenant targets must be fixed HTTPS origins")
			}
			if owner := targets[raw]; owner != "" && owner != t.ID {
				return nil, errors.New("tenants cannot share a plane origin")
			}
			targets[raw] = t.ID
			tenantID := t.ID
			proxy := &httputil.ReverseProxy{Transport: client.Transport, FlushInterval: -1, Rewrite: func(p *httputil.ProxyRequest) {
				p.Out.Header.Del("X-vMCP-Tenant")
				p.SetURL(target)
				p.SetXForwarded()
				p.Out.Header.Set("X-vMCP-Tenant", tenantID)
			}, ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				http.Error(w, "tenant service unavailable", 502)
			}}
			r.routes[t.Host][kind] = proxy
		}
	}
	return r, nil
}
func (r *Router) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	host := strings.ToLower(request.Host)
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	routes := r.routes[host]
	if routes == nil {
		http.NotFound(w, request)
		return
	}
	kind := "ui"
	if strings.HasPrefix(request.URL.Path, "/mcp/") {
		kind = "data"
	} else if strings.HasPrefix(request.URL.Path, "/api/") || strings.HasPrefix(request.URL.Path, "/auth/") {
		kind = "control"
	}
	routes[kind].ServeHTTP(w, request)
}
