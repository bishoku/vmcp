package controlplane

import (
	"github.com/bishoku/vmcp/core/internal/store"
	"net/http"
)

func (s *Server) teams(w http.ResponseWriter, r *http.Request) error {
	if r.Method == http.MethodGet {
		v, err := s.Store.Reader().ListTeams(r.Context())
		if err != nil {
			return err
		}
		return writeJSON(w, 200, map[string]any{"items": v})
	}
	var v store.Team
	if err := decode(r, &v); err != nil {
		return err
	}
	if err := s.Store.Do(r.Context(), func(t *store.Tx) error { return t.PutTeam(r.Context(), v) }); err != nil {
		return err
	}
	return writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) projects(w http.ResponseWriter, r *http.Request) error {
	if r.Method == http.MethodGet {
		v, err := s.Store.Reader().ListProjects(r.Context())
		if err != nil {
			return err
		}
		return writeJSON(w, 200, map[string]any{"items": v})
	}
	var v store.Project
	if err := decode(r, &v); err != nil {
		return err
	}
	if err := s.Store.Do(r.Context(), func(t *store.Tx) error { return t.PutProject(r.Context(), v) }); err != nil {
		return err
	}
	return writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) toolGrants(w http.ResponseWriter, r *http.Request) error {
	if r.Method == http.MethodGet {
		v, err := s.Store.Reader().ListToolGrants(r.Context())
		if err != nil {
			return err
		}
		return writeJSON(w, 200, map[string]any{"items": v})
	}
	var v store.ToolGrant
	if err := decode(r, &v); err != nil {
		return err
	}
	if err := s.Store.Do(r.Context(), func(t *store.Tx) error { return t.PutToolGrant(r.Context(), v) }); err != nil {
		return err
	}
	return writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) toolCatalog(w http.ResponseWriter, r *http.Request) error {
	limit, after, _, err := pageParams(r)
	if err != nil {
		return err
	}
	v, more, err := s.Store.Reader().ToolCatalog(r.Context(), after, limit)
	if err != nil {
		return err
	}
	var next *string
	if more {
		last := v[len(v)-1]
		next = cursorFor(last.Upstream + "/" + last.Name)
	}
	return writeJSON(w, 200, map[string]any{"items": v, "nextCursor": next})
}

func (s *Server) bulkToolGrants(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Items []store.ToolGrant `json:"items"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	result, err := s.Store.BulkToolGrants(r.Context(), body.Items)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, result)
}
