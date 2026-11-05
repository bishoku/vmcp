package controlplane

import (
	"github.com/bishoku/vmcp/core/internal/authz"
	"github.com/bishoku/vmcp/core/internal/store"
	"net/http"
)

func (s *Server) authorizationTrustedIssuer() string {
	if s.OIDC != nil {
		return s.OIDC.Issuer()
	}
	return ""
}
func (s *Server) authorizationMappings(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	if r.Method == "GET" {
		if id != "" {
			v, err := s.Store.Reader().AuthorizationMapping(r.Context(), id)
			if err != nil {
				return err
			}
			setETag(w, v.Version)
			return writeJSON(w, 200, v)
		}
		limit, after, enabled, err := pageParams(r)
		if err != nil {
			return err
		}
		p, err := store.LoadAuthorization(r.Context(), s.Store.Pool)
		if err != nil {
			return err
		}
		items := []authz.Mapping{}
		var next *string
		for _, m := range p.Mappings {
			if m.ID <= after || enabled != nil && m.Enabled != *enabled {
				continue
			}
			if len(items) == limit {
				next = cursorFor(items[len(items)-1].ID)
				break
			}
			items = append(items, m)
		}
		return writeJSON(w, 200, map[string]any{"items": items, "nextCursor": next, "authorizationRevision": p.Revision})
	}
	var expected *int64
	if r.Method != "POST" {
		v, err := requiredAuthorizationMatch(r)
		if err != nil {
			return err
		}
		expected = &v
	}
	if r.Method == "DELETE" {
		if err := s.Store.Do(r.Context(), func(t *store.Tx) error { return t.DeleteAuthorizationMapping(r.Context(), id, *expected) }); err != nil {
			return err
		}
		w.WriteHeader(204)
		return nil
	}
	var in store.MappingInput
	if err := decode(r, &in); err != nil {
		return err
	}
	var out authz.Mapping
	err := s.Store.Do(r.Context(), func(t *store.Tx) error {
		var err error
		out, err = t.SaveAuthorizationMapping(r.Context(), id, in, expected, s.authorizationTrustedIssuer())
		return err
	})
	if err != nil {
		return err
	}
	setETag(w, out.Version)
	status := 200
	if r.Method == "POST" {
		status = 201
		w.Header().Set("Location", "/api/v1/identity-mappings/"+out.ID)
	}
	return writeJSON(w, status, out)
}
func (s *Server) evaluateAuthorization(w http.ResponseWriter, r *http.Request) error {
	var in store.AuthorizationEvaluationInput
	if err := decode(r, &in); err != nil {
		return err
	}
	out, err := s.Store.EvaluateAuthorization(r.Context(), in, s.authorizationTrustedIssuer())
	if err != nil {
		return err
	}
	if s.OIDC != nil && s.OIDC.ClaimSessionsEnabled() {
		out.ClaimMappings = "live-audit"
	}
	return writeJSON(w, 200, out)
}
func (s *Server) authorizationStatus(w http.ResponseWriter, r *http.Request) error {
	out, err := s.Store.AuthorizationStatus(r.Context(), s.authorizationTrustedIssuer())
	if err != nil {
		return err
	}
	if s.OIDC != nil && s.OIDC.ClaimSessionsEnabled() {
		out.ClaimMappings = "live-audit"
		blockers := []string{}
		for _, b := range out.Blockers {
			if b != "oidc_claim_sessions_not_connected" {
				blockers = append(blockers, b)
			}
		}
		out.Blockers = blockers
	}
	return writeJSON(w, 200, out)
}
