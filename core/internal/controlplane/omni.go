package controlplane

import (
	"errors"
	"net/http"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/omni"
)

func (s *Server) omni(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "no-store")
	url := config.Env("VMCP_OMNI_URL", "")
	endpoint := config.Env("VMCP_OMNI_PUBLIC_URL", "")
	p, _ := r.Context().Value(principalKey{}).(*principal)
	admin := p != nil && p.Role == "admin"
	if url == "" {
		if r.Method == "GET" {
			return writeJSON(w, 200, map[string]any{"configured": false, "endpoint": endpoint})
		}
		return apierror.New(503, "omni_unavailable", "Omni unavailable", "Omni is not configured")
	}
	if r.Method == "GET" && !admin {
		return writeJSON(w, 200, map[string]any{"configured": true, "endpoint": endpoint})
	}
	b, err := omni.NewBackend(url, config.Env("VMCP_OMNI_SECRET", ""))
	if err != nil {
		return apierror.New(503, "omni_unavailable", "Omni unavailable", "Invalid Omni configuration")
	}
	path := "/internal/status"
	var body any
	token := ""
	switch r.URL.Path {
	case "/api/v1/omni/reindex":
		path = "/internal/reindex"
	case "/api/v1/omni/preview":
		path = "/internal/preview"
		var q struct {
			Query string `json:"query"`
			Ref   string `json:"tool_ref"`
		}
		if err := decode(r, &q); err != nil {
			return err
		}
		body = q
		// Explicit runtime identity, never the management session. Headers are not audited as payloads.
		token = r.Header.Get("X-VMCP-Runtime-Authorization")
		if token == "" {
			return apierror.New(400, "runtime_identity_required", "Runtime identity required", "Provide an agent key or OAuth access token")
		}
	}
	var out any
	if err = b.Request(r.Context(), r.Method, path, token, s.clientIPString(r), r.Header.Get("traceparent"), body, &out); err != nil {
		var e *omni.HTTPError
		if errors.As(err, &e) && (e.Status == 401 || e.Status == 403) {
			return apierror.Forbidden("runtime identity cannot access the requested Omni catalog")
		}
		return apierror.New(503, "omni_unavailable", "Omni unavailable", "Omni request could not be completed")
	}
	if r.Method == "GET" {
		return writeJSON(w, 200, map[string]any{"configured": true, "endpoint": endpoint, "index": out})
	}
	return writeJSON(w, 200, out)
}
