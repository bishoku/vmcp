package controlplane

import (
	"errors"
	"github.com/bishoku/vmcp/core/internal/admission"
	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/store"
	"net/http"
)

func (s *Server) getAdmissionSettings(w http.ResponseWriter, r *http.Request) error {
	settings, err := store.LoadAdmissionSettings(r.Context(), s.Store.Pool)
	if err != nil {
		return err
	}
	return writeJSON(w, 200, settings)
}
func (s *Server) putAdmissionSettings(w http.ResponseWriter, r *http.Request) error {
	var settings admission.Settings
	if err := decode(r, &settings); err != nil {
		return err
	}
	if err := settings.Validate(); err != nil {
		return apierror.New(422, "invalid_admission_settings", "Invalid admission settings", err.Error())
	}
	err := s.Store.Do(r.Context(), func(tx *store.Tx) error { return tx.SaveAdmissionSettings(r.Context(), settings, settings.Revision) })
	if errors.Is(err, store.ErrAdmissionSettingsConflict) {
		return apierror.New(409, "revision_conflict", "Settings changed", "Reload admission settings before saving")
	}
	if err != nil {
		return err
	}
	return s.getAdmissionSettings(w, r)
}
