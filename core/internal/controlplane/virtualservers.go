package controlplane

import (
	"net/http"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/pii"
	"github.com/bishoku/vmcp/core/internal/policy"
	"github.com/bishoku/vmcp/core/internal/store"
)

type virtualServerWire struct {
	Slug         string                `json:"slug"`
	Name         *string               `json:"name"`
	Description  *string               `json:"description"`
	Enabled      *bool                 `json:"enabled"`
	AuthRequired *bool                 `json:"authRequired"`
	PII          *pii.Config           `json:"pii"`
	Policy       *policy.Config        `json:"policy"`
	RateLimit    *int                  `json:"rateLimitPerMinute"`
	Tools        []store.ToolSelection `json:"tools"`
}

func (w *virtualServerWire) input() (store.VirtualServerInput, []apierror.FieldError) {
	in := store.VirtualServerInput{Enabled: true, AuthRequired: true, Tools: w.Tools}
	var errs []apierror.FieldError
	if w.Name == nil {
		errs = append(errs, required("/name"))
	} else {
		in.Name = *w.Name
	}
	if w.Description != nil {
		in.Description = *w.Description
	}
	if w.Enabled != nil {
		in.Enabled = *w.Enabled
	}
	if w.AuthRequired != nil {
		in.AuthRequired = *w.AuthRequired
	}
	if w.PII != nil {
		in.PII = *w.PII
	}
	if w.Policy != nil {
		in.Policy = *w.Policy
	}
	in.RateLimit = w.RateLimit
	return in, errs
}

type virtualServerPatchWire struct {
	Name         *string                `json:"name"`
	Description  *string                `json:"description"`
	Enabled      *bool                  `json:"enabled"`
	AuthRequired *bool                  `json:"authRequired"`
	PII          *pii.Config            `json:"pii"`
	Policy       *policy.Config         `json:"policy"`
	RateLimit    optionalInt            `json:"rateLimitPerMinute"`
	Tools        *[]store.ToolSelection `json:"tools"`
}

func (s *Server) listVirtualServers(w http.ResponseWriter, r *http.Request) error {
	limit, after, enabled, err := pageParams(r)
	if err != nil {
		return err
	}
	items, more, err := s.Store.Reader().ListVirtualServers(r.Context(), after, limit, enabled)
	if err != nil {
		return err
	}
	var next *string
	if more {
		next = cursorFor(items[len(items)-1].Slug)
	}
	return writeJSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": next})
}

func (s *Server) getVirtualServer(w http.ResponseWriter, r *http.Request) error {
	rd := s.Store.Reader()
	slug := r.PathValue("slug")
	v, err := rd.GetVirtualServer(r.Context(), slug)
	if err != nil {
		return err
	}
	ver, err := rd.VirtualServerVersion(r.Context(), slug)
	if err != nil {
		return err
	}
	setETag(w, ver)
	return writeJSON(w, http.StatusOK, v)
}

func (s *Server) createVirtualServer(w http.ResponseWriter, r *http.Request) error {
	var body virtualServerWire
	if err := decode(r, &body); err != nil {
		return err
	}
	in, errs := body.input()
	if body.Slug == "" {
		errs = append(errs, required("/slug"))
	}
	if len(errs) > 0 {
		return apierror.Validation(errs)
	}
	return s.writeVirtualServer(w, r, body.Slug, in, nil, true)
}

func (s *Server) putVirtualServer(w http.ResponseWriter, r *http.Request) error {
	slug := r.PathValue("slug")
	var body virtualServerWire
	if err := decode(r, &body); err != nil {
		return err
	}
	in, errs := body.input()
	if body.Slug != "" && body.Slug != slug {
		errs = append(errs, apierror.FieldError{Field: "/slug", Code: apierror.CodeValidationFailed, Message: "must match the slug in the URL"})
	}
	if len(errs) > 0 {
		return apierror.Validation(errs)
	}
	ifMatch, err := parseIfMatch(r)
	if err != nil {
		return err
	}
	return s.writeVirtualServer(w, r, slug, in, ifMatch, false)
}

func (s *Server) writeVirtualServer(w http.ResponseWriter, r *http.Request, slug string, in store.VirtualServerInput, ifMatch *int64, createOnly bool) error {
	var (
		v       *store.VirtualServer
		outcome store.Outcome
		version int64
	)
	err := s.Store.Do(r.Context(), func(tx *store.Tx) error {
		var err error
		if v, outcome, err = tx.PutVirtualServer(r.Context(), slug, in, ifMatch, createOnly); err != nil {
			return err
		}
		version, err = tx.VirtualServerVersion(r.Context(), slug)
		return err
	})
	if err != nil {
		return err
	}
	setETag(w, version)
	s.setRevision(w, r)
	status := http.StatusOK
	if outcome == store.Created {
		status = http.StatusCreated
		w.Header().Set("Location", "/api/v1/virtual-servers/"+slug)
	}
	return writeJSON(w, status, v)
}

func (s *Server) patchVirtualServer(w http.ResponseWriter, r *http.Request) error {
	slug := r.PathValue("slug")
	var body virtualServerPatchWire
	if err := decode(r, &body); err != nil {
		return err
	}
	ifMatch, err := parseIfMatch(r)
	if err != nil {
		return err
	}
	var (
		v       *store.VirtualServer
		version int64
	)
	err = s.Store.Do(r.Context(), func(tx *store.Tx) error {
		var err error
		v, err = tx.PatchVirtualServer(r.Context(), slug, store.VirtualServerPatch{
			Name: body.Name, Description: body.Description, Enabled: body.Enabled, AuthRequired: body.AuthRequired, PII: body.PII, Policy: body.Policy, RateLimitSet: body.RateLimit.Set, RateLimit: body.RateLimit.Value, Tools: body.Tools,
		}, ifMatch)
		if err != nil {
			return err
		}
		version, err = tx.VirtualServerVersion(r.Context(), slug)
		return err
	})
	if err != nil {
		return err
	}
	setETag(w, version)
	s.setRevision(w, r)
	return writeJSON(w, http.StatusOK, v)
}

func (s *Server) deleteVirtualServer(w http.ResponseWriter, r *http.Request) error {
	ifMatch, err := parseIfMatch(r)
	if err != nil {
		return err
	}
	if err := s.Store.Do(r.Context(), func(tx *store.Tx) error {
		return tx.DeleteVirtualServer(r.Context(), r.PathValue("slug"), ifMatch)
	}); err != nil {
		return err
	}
	s.setRevision(w, r)
	w.WriteHeader(http.StatusNoContent)
	return nil
}
