package controlplane

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/audit"
)

// This is an explicit allowlist, not a prefix inference for arbitrary new routes.
// Creates, identity/credential management, transfers, network operations and
// tenant-wide documents must never inherit a scope from a client-supplied body.
func controlAuditTarget(r *http.Request) (kind string) {
	switch r.Pattern {
	case "GET /api/v1/upstreams/{slug}/invocation-security", "PUT /api/v1/upstreams/{slug}/invocation-security":
		return "upstream"
	case "GET /api/v1/upstreams/{slug}/audit-settings", "PUT /api/v1/upstreams/{slug}/audit-settings", "POST /api/v1/upstreams/{slug}/audit-settings/preview":
		return "upstream"
	case "GET /api/v1/virtual-servers/{slug}/audit-settings", "PUT /api/v1/virtual-servers/{slug}/audit-settings", "POST /api/v1/virtual-servers/{slug}/audit-settings/preview":
		return "virtual_server"
	case "GET /api/v1/upstreams/{slug}", "GET /api/v1/upstreams/{slug}/tools", "GET /api/v1/upstreams/{slug}/catalog", "PUT /api/v1/upstreams/{slug}", "PATCH /api/v1/upstreams/{slug}", "DELETE /api/v1/upstreams/{slug}":
		return "upstream"
	case "GET /api/v1/virtual-servers/{slug}", "PUT /api/v1/virtual-servers/{slug}", "PATCH /api/v1/virtual-servers/{slug}", "DELETE /api/v1/virtual-servers/{slug}":
		return "virtual_server"
	}
	return ""
}

func (s *Server) pinControlAudit(r *http.Request, capture bool) (*audit.AccessContext, []byte, error) {
	kind := controlAuditTarget(r)
	var raw []byte
	// Inspect bounded top-level keys even when capture is off: classification is
	// independent of retention policy. Preserve the handler's strict decoder.
	inspect := kind != "" && (r.Method == "PUT" || r.Method == "PATCH")
	if (capture || inspect) && r.Body != nil {
		var err error
		raw, err = io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
		if err != nil {
			return nil, nil, apierror.New(400, "invalid_body", "Invalid body", "request could not be read")
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
	}
	if inspect {
		var object map[string]json.RawMessage
		if json.Unmarshal(raw, &object) != nil || object == nil {
			kind = ""
		}
		for key := range object {
			// encoding/json accepts case-insensitive matches for these struct fields.
			if strings.EqualFold(key, "ownerTeam") || strings.EqualFold(key, "project") {
				kind = ""
			}
		}
	}
	access, err := s.Store.ControlAuditContext(r.Context(), kind, r.PathValue("slug"))
	if err != nil {
		return nil, nil, err
	}
	if !capture {
		raw = nil
	}
	return access, raw, nil
}
