package controlplane

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/store"
)

type configurationProbeKey struct{}
type configurationProbe struct {
	operation store.ConfigurationOperation
	known     bool
}

// Self-only affordances. This is neither an assignment simulator nor an authority
// token: the real endpoint always authorizes again against its current policy.
func (s *Server) configurationAccess(probe *http.ServeMux) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		w.Header().Set("Cache-Control", "no-store")
		var in struct {
			Checks []struct {
				Method     string         `json:"method"`
				Path       string         `json:"path"`
				Body       map[string]any `json:"body,omitempty"`
				Permission string         `json:"permission,omitempty"`
			} `json:"checks"`
		}
		if err := decode(r, &in); err != nil {
			return err
		}
		if len(in.Checks) > 64 {
			return apierror.Validation([]apierror.FieldError{{Message: "at most 64 checks allowed"}})
		}
		policy, err := store.LoadAuthorization(r.Context(), s.Store.Pool)
		if err != nil {
			return err
		}
		p := r.Context().Value(principalKey{}).(*principal)
		policy = (payloadPolicy{Policy: policy}).effective(p)
		allowed := make([]bool, len(in.Checks))
		for i, check := range in.Checks {
			u, e := url.Parse(check.Path)
			if e != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(check.Path, "/") || strings.HasPrefix(check.Path, "//") {
				continue
			}
			target := &configurationProbe{}
			req, e := http.NewRequestWithContext(context.WithValue(r.Context(), configurationProbeKey{}, target), check.Method, "http://configuration.local/api/v1"+check.Path, nil)
			if e != nil {
				continue
			}
			probe.ServeHTTP(discardConfigurationResponse{}, req)
			if !target.known || target.operation.Family == "self" {
				continue
			}
			op := target.operation
			op.Body = check.Body
			op.GlobalAdmin = p.Role == "admin"
			if check.Permission != "" {
				allowed[i] = s.Store.Reader().ConfigurationPermission(r.Context(), policy, payloadPrincipal(p), op, check.Permission) == nil
			} else {
				allowed[i] = s.Store.Reader().CheckConfiguration(r.Context(), policy, payloadPrincipal(p), op) == nil
				if allowed[i] && op.Family == "tool_grants" && op.Action == "bulk" {
					items, ok := op.Body["items"].([]any)
					if !ok || len(items) > 200 {
						allowed[i] = false
						continue
					}
					for _, item := range items {
						body, ok := item.(map[string]any)
						if !ok {
							allowed[i] = false
							break
						}
						itemOp := store.ConfigurationOperation{Family: "tool_grants", Verb: "create", Body: body, GlobalAdmin: p.Role == "admin"}
						if s.Store.Reader().CheckConfiguration(r.Context(), policy, payloadPrincipal(p), itemOp) != nil {
							allowed[i] = false
							break
						}
					}
				}
			}
		}
		return writeJSON(w, 200, map[string]any{"authorizationRevision": policy.Revision, "allowed": allowed})
	}
}

type discardConfigurationResponse struct{}

func (discardConfigurationResponse) Header() http.Header         { return http.Header{} }
func (discardConfigurationResponse) Write(b []byte) (int, error) { return len(b), nil }
func (discardConfigurationResponse) WriteHeader(int)             {}
