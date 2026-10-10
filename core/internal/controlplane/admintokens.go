package controlplane

import (
	"net/http"
	"time"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/store"
)

type adminTokenWire struct {
	Slug        string     `json:"slug"`
	Description *string    `json:"description"`
	Scope       *string    `json:"scope"`
	ExpiresAt   *time.Time `json:"expiresAt"`
}

type adminTokenPatchWire struct {
	Description *string      `json:"description"`
	Scope       *string      `json:"scope"`
	ExpiresAt   optionalTime `json:"expiresAt"`
}

type adminWithSecret struct {
	*store.AdminToken
	Token string `json:"token"`
}

func (s *Server) listAdminTokens(w http.ResponseWriter, r *http.Request) error {
	limit, after, _, err := pageParams(r)
	if err != nil {
		return err
	}
	items, more, err := s.Store.Reader().ListAdminTokens(r.Context(), after, limit)
	if err != nil {
		return err
	}
	var next *string
	if more {
		next = cursorFor(items[len(items)-1].Slug)
	}
	return writeJSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": next})
}

func (s *Server) getAdminToken(w http.ResponseWriter, r *http.Request) error {
	rd := s.Store.Reader()
	a, err := rd.GetAdminToken(r.Context(), r.PathValue("slug"))
	if err != nil {
		return err
	}
	setETag(w, a.Version)
	return writeJSON(w, http.StatusOK, a)
}

func (s *Server) createAdminToken(w http.ResponseWriter, r *http.Request) error {
	var body adminTokenWire
	if err := decode(r, &body); err != nil {
		return err
	}
	if body.Slug == "" {
		return apierror.Validation([]apierror.FieldError{required("/slug")})
	}
	in := store.AdminTokenInput{ExpiresAt: body.ExpiresAt}
	if body.Description != nil {
		in.Description = *body.Description
	}
	if body.Scope != nil {
		in.Scope = *body.Scope
	}
	var (
		a      *store.AdminToken
		secret string
	)
	if err := s.Store.Do(r.Context(), func(tx *store.Tx) error {
		var err error
		a, secret, err = tx.CreateAdminToken(r.Context(), body.Slug, in)
		return err
	}); err != nil {
		return err
	}
	w.Header().Set("Location", "/api/v1/admin-tokens/"+body.Slug)
	w.Header().Set("Cache-Control", "no-store")
	return writeJSON(w, http.StatusCreated, adminWithSecret{a, secret})
}

func (s *Server) patchAdminToken(w http.ResponseWriter, r *http.Request) error {
	var body adminTokenPatchWire
	if err := decode(r, &body); err != nil {
		return err
	}
	ifMatch, err := parseIfMatch(r)
	if err != nil {
		return err
	}
	var a *store.AdminToken
	if err := s.Store.Do(r.Context(), func(tx *store.Tx) error {
		var err error
		a, err = tx.PatchAdminToken(r.Context(), r.PathValue("slug"), store.AdminTokenPatch{
			Description: body.Description, Scope: body.Scope, ExpiresAtSet: body.ExpiresAt.Set, ExpiresAt: body.ExpiresAt.Value,
		}, ifMatch)
		return err
	}); err != nil {
		return err
	}
	setETag(w, a.Version)
	return writeJSON(w, http.StatusOK, a)
}

func (s *Server) deleteAdminToken(w http.ResponseWriter, r *http.Request) error {
	ifMatch, err := parseIfMatch(r)
	if err != nil {
		return err
	}
	if err := s.Store.Do(r.Context(), func(tx *store.Tx) error {
		return tx.DeleteAdminToken(r.Context(), r.PathValue("slug"), ifMatch)
	}); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) rotateAdminToken(w http.ResponseWriter, r *http.Request) error {
	var (
		a      *store.AdminToken
		secret string
	)
	if err := s.Store.Do(r.Context(), func(tx *store.Tx) error {
		var err error
		a, secret, err = tx.RotateAdminToken(r.Context(), r.PathValue("slug"))
		return err
	}); err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "no-store")
	return writeJSON(w, http.StatusOK, adminWithSecret{a, secret})
}

func (s *Server) revokeAdminToken(w http.ResponseWriter, r *http.Request) error {
	var a *store.AdminToken
	if err := s.Store.Do(r.Context(), func(tx *store.Tx) error {
		var err error
		a, err = tx.RevokeAdminToken(r.Context(), r.PathValue("slug"))
		return err
	}); err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, a)
}
