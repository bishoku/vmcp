package controlplane

import (
	"net/http"
	"strings"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/authz"
	"github.com/bishoku/vmcp/core/internal/store"
)

func authorizationManagementRoute(r *http.Request) bool {
	p := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	return p == "permissions" || p == "roles" || strings.HasPrefix(p, "roles/") || p == "role-assignments" || strings.HasPrefix(p, "role-assignments/") || p == "identity-mappings" || strings.HasPrefix(p, "identity-mappings/") || strings.HasPrefix(p, "access/")
}
func requiredAuthorizationMatch(r *http.Request) (int64, error) {
	v, err := parseIfMatch(r)
	if err != nil {
		return 0, err
	}
	if v == nil {
		return 0, apierror.New(428, "precondition_required", "Precondition required", "an explicit If-Match version is required")
	}
	return *v, nil
}
func (s *Server) authorizationPermissions(w http.ResponseWriter, r *http.Request) error {
	return writeJSON(w, 200, map[string]any{"items": authz.Catalog()})
}
func (s *Server) authorizationRoles(w http.ResponseWriter, r *http.Request) error {
	slug := r.PathValue("slug")
	if r.Method == "GET" {
		if slug != "" {
			v, err := s.Store.Reader().AuthorizationRole(r.Context(), slug)
			if err != nil {
				return err
			}
			setETag(w, v.Version)
			return writeJSON(w, 200, v)
		}
		p, err := store.LoadAuthorization(r.Context(), s.Store.Pool)
		if err != nil {
			return err
		}
		return writeJSON(w, 200, map[string]any{"items": p.Roles, "authorizationRevision": p.Revision})
	}
	var match *int64
	if r.Method != "POST" {
		v, err := requiredAuthorizationMatch(r)
		if err != nil {
			return err
		}
		match = &v
	}
	if r.Method == "DELETE" {
		err := s.Store.Do(r.Context(), func(t *store.Tx) error { return t.DeleteAuthorizationRole(r.Context(), slug, *match) })
		if err != nil {
			return err
		}
		w.WriteHeader(204)
		return nil
	}
	var in store.RoleInput
	if err := decode(r, &in); err != nil {
		return err
	}
	var out authz.Role
	err := s.Store.Do(r.Context(), func(t *store.Tx) error {
		var err error
		out, err = t.SaveAuthorizationRole(r.Context(), slug, in, match)
		return err
	})
	if err != nil {
		return err
	}
	setETag(w, out.Version)
	status := 200
	if r.Method == "POST" {
		status = 201
		w.Header().Set("Location", "/api/v1/roles/"+out.Slug)
	}
	return writeJSON(w, status, out)
}

func (s *Server) authorizationAssignments(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	if r.Method == "GET" {
		if id != "" {
			v, err := s.Store.Reader().AuthorizationAssignment(r.Context(), id)
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
		items := []authz.Assignment{}
		var next *string
		for _, a := range p.Assignments {
			if a.ID <= after || enabled != nil && a.Enabled != *enabled {
				continue
			}
			if len(items) == limit {
				next = cursorFor(items[len(items)-1].ID)
				break
			}
			items = append(items, a)
		}
		return writeJSON(w, 200, map[string]any{"items": items, "nextCursor": next, "authorizationRevision": p.Revision})
	}
	var match *int64
	if r.Method != "POST" {
		v, err := requiredAuthorizationMatch(r)
		if err != nil {
			return err
		}
		match = &v
	}
	if r.Method == "DELETE" {
		if err := s.Store.Do(r.Context(), func(t *store.Tx) error { return t.DeleteAuthorizationAssignment(r.Context(), id, *match) }); err != nil {
			return err
		}
		w.WriteHeader(204)
		return nil
	}
	var in store.AssignmentInput
	if err := decode(r, &in); err != nil {
		return err
	}
	issuer := ""
	if s.OIDC != nil {
		issuer = s.OIDC.Issuer()
	}
	var out authz.Assignment
	err := s.Store.Do(r.Context(), func(t *store.Tx) error {
		var err error
		out, err = t.SaveAuthorizationAssignment(r.Context(), id, in, match, issuer)
		return err
	})
	if err != nil {
		return err
	}
	setETag(w, out.Version)
	status := 200
	if r.Method == "POST" {
		status = 201
		w.Header().Set("Location", "/api/v1/role-assignments/"+out.ID)
	}
	return writeJSON(w, status, out)
}

func (s *Server) authorizationScopeOptions(w http.ResponseWriter, r *http.Request) error {
	limit, after, _, err := pageParams(r)
	if err != nil {
		return err
	}
	items, more, err := s.Store.Reader().AuthorizationScopeOptions(r.Context(), r.URL.Query().Get("kind"), after, limit)
	if err != nil {
		return err
	}
	var next *string
	if more {
		next = cursorFor(items[len(items)-1].ID)
	}
	return writeJSON(w, 200, map[string]any{"items": items, "nextCursor": next})
}
func (s *Server) authorizationResourceEpochs(w http.ResponseWriter, r *http.Request) error {
	limit, after, _, err := pageParams(r)
	if err != nil {
		return err
	}
	items, more, err := s.Store.Reader().AuthorizationResourceEpochs(r.Context(), r.URL.Query().Get("kind"), r.URL.Query().Get("resourceId"), after, limit)
	if err != nil {
		return err
	}
	var next *string
	if more {
		next = cursorFor(items[len(items)-1].ID)
	}
	return writeJSON(w, 200, map[string]any{"items": items, "nextCursor": next})
}
