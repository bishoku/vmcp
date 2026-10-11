package controlplane

import "net/http"

func (s *Server) listUpstreamCatalog(w http.ResponseWriter, r *http.Request) error {
	catalog, err := s.Store.Reader().ListCatalog(r.Context(), r.PathValue("slug"))
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, catalog)
}
