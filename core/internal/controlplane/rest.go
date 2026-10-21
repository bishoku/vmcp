package controlplane

import (
	"encoding/json"
	"net/http"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/callercontext"
	"github.com/bishoku/vmcp/core/internal/restapi"
)

func (s *Server) restOperation(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Operation string          `json:"operation"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	conn, err := s.Store.Reader().UpstreamConnInfo(r.Context(), r.PathValue("slug"))
	if err != nil {
		return err
	}
	if conn.Kind != "rest" || conn.REST == nil {
		return apierror.Validation([]apierror.FieldError{storeRestError("REST upstream required")})
	}
	compiled, err := restapi.Compile(conn.URL, *conn.REST, conn.Headers)
	if err != nil {
		return apierror.Validation([]apierror.FieldError{storeRestError("Invalid saved REST configuration")})
	}
	if r.PathValue("action") == "preview" {
		preview, err := compiled.Preview(conn.URL, body.Operation, body.Arguments, conn.Headers)
		if err != nil {
			return apierror.Validation([]apierror.FieldError{storeRestError(err.Error())})
		}
		if c := conn.CallerContext; c != nil && c.Enabled {
			for _, f := range c.Fields {
				preview.Headers[f.Header] = "[caller context: " + f.Name + "]"
			}
		}
		for _, rule := range conn.RequestHeaderMappings {
			preview.Headers[rule.Target] = "[request header: " + rule.Source + "]"
		}
		return writeJSON(w, 200, preview)
	}
	if r.PathValue("action") != "test" {
		return apierror.NotFound("REST action", r.PathValue("action"))
	}
	principal, _ := r.Context().Value(principalKey{}).(*principal)
	if principal == nil || principal.Role != "admin" {
		return apierror.Forbidden("REST execution tests require admin access")
	}
	if !conn.Enabled {
		return apierror.Forbidden("Enable the upstream before executing a test")
	}
	match, err := parseIfMatch(r)
	if err != nil {
		return err
	}
	if match == nil || *match != conn.Version {
		return apierror.PreconditionFailed()
	}
	if _, _, err := conn.CallerContext.Prepare(nil, "rest"); err != nil {
		return apierror.Validation([]apierror.FieldError{storeRestError("Required caller context must be tested through a tool call with _meta")})
	}
	if _, err := callercontext.PrepareHeaders(conn.RequestHeaderMappings, nil); err != nil {
		return apierror.Validation([]apierror.FieldError{storeRestError("Required request headers must be tested through an MCP tool call")})
	}
	// Admin tests do not impersonate an agent. Identity-required connections must be tested through MCP.
	result, err := compiled.Execute(r.Context(), conn.URL, body.Operation, body.Arguments, conn.Headers, conn.TLS, restapi.Identity{}, nil)
	if err != nil {
		return apierror.New(502, apierror.CodeUpstreamUnreach, "REST test failed", "Request failed or timed out; operation may have completed")
	}
	return writeJSON(w, 200, result)
}

// importREST is a bounded, non-persisting preview; the normal save transaction
// remains the sole authority for REST validation, secret retention and publication.
func (s *Server) importREST(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Document string `json:"document"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	result, err := restapi.Import(body.Document)
	if err != nil {
		return apierror.Validation([]apierror.FieldError{{Field: "/document", Code: apierror.CodeValidationFailed, Message: err.Error()}})
	}
	return writeJSON(w, http.StatusOK, result)
}
