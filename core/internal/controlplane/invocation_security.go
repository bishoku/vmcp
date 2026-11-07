package controlplane

import (
	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/invocation"
	"github.com/bishoku/vmcp/core/internal/store"
	"net/http"
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
	for name, svc := range settings.Services {
		svc.HasToken = svc.Token != ""
		svc.Token = ""
		settings.Services[name] = svc
	}
	return writeJSON(w, 200, settings)
}
