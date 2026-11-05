package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/store"
)

type localAuditDocument struct {
	Revision int64        `json:"revision"`
	Rules    []audit.Rule `json:"rules"`
}

func localAuditRules(settings audit.Settings, kind, slug string) []audit.Rule {
	rules := settings.Upstreams[slug]
	if kind == "virtual_server" {
		rules = settings.VirtualServers[slug]
	}
	return append([]audit.Rule{}, rules...)
}
func setLocalAuditRules(settings *audit.Settings, kind, slug string, rules []audit.Rule) {
	if kind == "virtual_server" {
		settings.VirtualServers[slug] = rules
	} else {
		settings.Upstreams[slug] = rules
	}
}
func localSettingsConflict(err error) error {
	if errors.Is(err, store.ErrAuditSettingsConflict) {
		return apierror.New(409, "revision_conflict", "Settings changed", "Reload local audit settings before continuing")
	}
	return err
}
func (s *Server) localAuditSettings(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "no-store")
	kind := "upstream"
	scope := "upstream"
	if r.Pattern == "GET /api/v1/virtual-servers/{slug}/audit-settings" || r.Pattern == "PUT /api/v1/virtual-servers/{slug}/audit-settings" || r.Pattern == "POST /api/v1/virtual-servers/{slug}/audit-settings/preview" {
		kind = "virtual_server"
		scope = "virtual"
	}
	slug := r.PathValue("slug")
	if r.Method == "GET" {
		settings, err := s.Store.Reader().AuditSettings(r.Context())
		if err != nil {
			return err
		}
		return writeJSON(w, 200, localAuditDocument{settings.Revision, localAuditRules(settings, kind, slug)})
	}
	if r.Method == "POST" {
		var body struct {
			Revision  int64           `json:"revision"`
			Rules     []audit.Rule    `json:"rules"`
			Direction string          `json:"direction"`
			Payload   json.RawMessage `json:"payload"`
		}
		if err := decode(r, &body); err != nil {
			return err
		}
		if body.Revision <= 0 || body.Rules == nil || len(body.Payload) == 0 {
			return apierror.New(422, "invalid_audit_settings", "Invalid preview", "revision, rules array and payload are required")
		}
		return s.Store.ConfigurationRead(r.Context(), func(ctx context.Context) error {
			settings, err := s.Store.Reader().AuditSettings(ctx)
			if err != nil {
				return err
			}
			if settings.Revision != body.Revision {
				return localSettingsConflict(store.ErrAuditSettingsConflict)
			}
			setLocalAuditRules(&settings, kind, slug, body.Rules)
			if err := settings.Validate(); err != nil {
				return apierror.New(422, "invalid_audit_settings", "Invalid local rules", err.Error())
			}
			vs, up := "", ""
			if kind == "virtual_server" {
				vs = slug
			} else {
				up = slug
			}
			preview, err := audit.PreviewPayload(settings, scope, body.Direction, vs, up, body.Payload)
			if err != nil {
				return apierror.New(422, "audit_preview_failed", "Preview failed", err.Error())
			}
			return writeJSON(w, 200, preview)
		})
	}
	var body localAuditDocument
	if err := decode(r, &body); err != nil {
		return err
	}
	if body.Revision <= 0 || body.Rules == nil {
		return apierror.New(422, "invalid_audit_settings", "Invalid local settings", "revision and rules array are required")
	}
	var saved localAuditDocument
	err := s.Store.Do(r.Context(), func(tx *store.Tx) error {
		settings, err := tx.AuditSettings(r.Context())
		if err != nil {
			return err
		}
		if settings.Revision != body.Revision {
			return store.ErrAuditSettingsConflict
		}
		setLocalAuditRules(&settings, kind, slug, body.Rules)
		if err := settings.Validate(); err != nil {
			return apierror.New(422, "invalid_audit_settings", "Invalid local rules", err.Error())
		}
		if err := tx.SaveAuditSettings(r.Context(), settings, body.Revision); err != nil {
			return err
		}
		saved = localAuditDocument{body.Revision + 1, localAuditRules(settings, kind, slug)}
		return nil
	})
	if err != nil {
		return localSettingsConflict(err)
	}
	return writeJSON(w, 200, saved)
}
