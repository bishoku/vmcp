package controlplane

import (
	"net/http"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/invocation"
	"github.com/bishoku/vmcp/core/internal/store"
)

func (s *Server) invocationSecurity(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == "PUT" {
		var settings invocation.Settings
		if err := decode(r, &settings); err != nil {
			return err
		}
		if err := settings.Validate(); err != nil {
			return apierror.New(422, "invalid_invocation_security", "Invalid invocation security", err.Error())
		}
		if err := s.Store.Do(r.Context(), func(tx *store.Tx) error { return tx.SaveInvocationSecurity(r.Context(), settings) }); err != nil {
			return err
		}
	}
	settings, err := s.Store.Reader().InvocationSecurity(r.Context())
	if err != nil {
		return err
	}
	settings.HideCredentials(true)
	return writeJSON(w, 200, settings)
}

func (s *Server) upstreamInvocationSecurity(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "no-store")
	ctx, slug := r.Context(), r.PathValue("slug")
	if r.Method == "GET" {
		doc, err := s.Store.Reader().UpstreamInvocationSecurity(ctx, slug)
		if err != nil {
			return err
		}
		return writeJSON(w, 200, doc)
	}
	var in struct {
		UpstreamID string                     `json:"upstreamId"`
		Revision   *int64                     `json:"revision"`
		Policy     *invocation.UpstreamPolicy `json:"policy"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.UpstreamID == "" || in.Revision == nil || *in.Revision < 0 || in.Policy == nil {
		return apierror.Validation([]apierror.FieldError{{Message: "upstreamId, revision and policy are required"}})
	}
	var saved store.UpstreamInvocationSecurity
	if err := s.Store.Do(ctx, func(tx *store.Tx) error {
		if err := tx.SaveUpstreamInvocationSecurity(ctx, slug, in.UpstreamID, *in.Revision, *in.Policy); err != nil {
			return err
		}
		var err error
		saved, err = tx.UpstreamInvocationSecurity(ctx, slug)
		return err
	}); err != nil {
		return err
	}
	return writeJSON(w, 200, saved)
}

func (s *Server) sharedInvocationServices(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "no-store")
	ctx := r.Context()
	if r.Method == "GET" {
		doc, err := s.Store.Reader().SharedInvocationServices(ctx)
		if err != nil {
			return err
		}
		return writeJSON(w, 200, doc)
	}
	var in struct {
		Revision int64                         `json:"revision"`
		Services map[string]invocation.Service `json:"services"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Revision < 1 || in.Services == nil {
		return apierror.Validation([]apierror.FieldError{{Message: "revision and services are required"}})
	}
	var saved store.SharedInvocationServices
	if err := s.Store.Do(ctx, func(tx *store.Tx) error {
		if err := tx.SaveSharedInvocationServices(ctx, in.Revision, in.Services); err != nil {
			return err
		}
		var err error
		saved, err = tx.SharedInvocationServices(ctx)
		return err
	}); err != nil {
		return err
	}
	return writeJSON(w, 200, saved)
}
