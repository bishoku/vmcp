package controlplane

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/jackc/pgx/v5"
)

func (s *Server) getAuditSettings(w http.ResponseWriter, r *http.Request) error {
	settings, err := store.LoadAuditSettings(r.Context(), s.Store.Pool)
	if err != nil {
		return err
	}
	return writeJSON(w, 200, settings)
}
func (s *Server) putAuditSettings(w http.ResponseWriter, r *http.Request) error {
	var settings audit.Settings
	if err := decode(r, &settings); err != nil {
		return err
	}
	settings = settings.Normalize()
	if err := settings.Validate(); err != nil {
		return apierror.New(422, "invalid_audit_settings", "Invalid audit settings", err.Error())
	}
	err := s.Store.Do(r.Context(), func(tx *store.Tx) error { return tx.SaveAuditSettings(r.Context(), settings, settings.Revision) })
	if errors.Is(err, store.ErrAuditSettingsConflict) {
		return apierror.New(409, "revision_conflict", "Settings changed", "Reload audit settings before saving")
	}
	if err != nil {
		return err
	}
	return s.getAuditSettings(w, r)
}
func (s *Server) previewAuditPayload(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		EditScope     string          `json:"editScope"`
		Settings      audit.Settings  `json:"settings"`
		Scope         string          `json:"scope"`
		Direction     string          `json:"direction"`
		VirtualServer string          `json:"virtualServer"`
		Upstream      string          `json:"upstream"`
		Payload       json.RawMessage `json:"payload"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	body.Settings = body.Settings.Normalize()
	if err := body.Settings.Validate(); err != nil {
		return apierror.New(422, "invalid_audit_settings", "Invalid audit settings", err.Error())
	}
	switch body.Scope {
	case "virtual", "upstream", "control", "system", "reads", "omni", "omni_diagnostics", "omni_embedding", "omni_rerank":
	default:
		return apierror.New(422, "invalid_scope", "Invalid scope", "Unknown capture scope")
	}
	if body.EditScope == "virtual" || body.EditScope == "upstream" {
		current, err := store.LoadAuditSettings(r.Context(), s.Store.Pool)
		if err != nil {
			return err
		}
		if current.Revision != body.Settings.Revision {
			return apierror.New(409, "revision_conflict", "Settings changed", "Reload audit settings before previewing")
		}
		if body.EditScope == "virtual" {
			current.VirtualServers[body.VirtualServer] = body.Settings.VirtualServers[body.VirtualServer]
		} else {
			current.Upstreams[body.Upstream] = body.Settings.Upstreams[body.Upstream]
		}
		body.Settings = current
	} else if body.EditScope != "" && body.EditScope != "global" {
		return apierror.New(422, "invalid_scope", "Invalid scope", "Unknown edit scope")
	}
	if strings.HasPrefix(body.Scope, "omni") {
		body.Settings = body.Settings.ForOmni()
	}
	p, err := audit.PreviewPayload(body.Settings, body.Scope, body.Direction, body.VirtualServer, body.Upstream, body.Payload)
	if err != nil {
		return apierror.New(422, "audit_preview_failed", "Preview failed", err.Error())
	}
	return writeJSON(w, 200, p)
}
func (s *Server) getAuditPayload(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("event")
	if !audit.ValidID(id) {
		return apierror.New(400, "invalid_event", "Invalid event", "event must be a UUID")
	}
	w.Header().Set("Cache-Control", "no-store")
	access, err := s.Store.AuditPayloadAccess(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		return payloadNotFound()
	}
	if err != nil {
		return apierror.New(503, "authorization_unavailable", "Authorization unavailable", "Audit event scope could not be verified")
	}
	policy, ok := r.Context().Value(payloadPolicyKey{}).(payloadPolicy)
	if !ok {
		return apierror.New(503, "authorization_unavailable", "Authorization unavailable", "Audit policy unavailable")
	}
	p, _ := r.Context().Value(principalKey{}).(*principal)
	if p == nil {
		return apierror.Unauthorized()
	}
	allowed := policy.canRead(p, access)
	status := "ok"
	if !allowed {
		status = "denied"
	}
	s.auditRecorder().Record(audit.Event{ActorType: p.Via, ActorID: p.ID, Issuer: p.Issuer, Subject: p.Subject, Method: r.Method, Action: "audit.payload.access", Target: "audit-event:" + id, Direction: access.Direction, Status: status, Reason: fmt.Sprintf("authorizationRevision=%d classification=%s", policy.Policy.Revision, access.Classification)})
	if !allowed {
		return payloadNotFound()
	}
	var state string
	var cipher []byte
	err = s.Store.Pool.QueryRow(r.Context(), `SELECT a.payload_state,p.ciphertext FROM audit_events a LEFT JOIN audit_payloads p ON p.event_id=a.event_id AND p.tenant_id=a.tenant_id WHERE a.event_id=$1::uuid AND a.tenant_id=vmcp_tenant_id()`, id).Scan(&state, &cipher)
	if errors.Is(err, pgx.ErrNoRows) {
		return apierror.New(404, "not_found", "Not found", "Audit event not found")
	}
	if err != nil {
		return err
	}
	var content json.RawMessage
	if len(cipher) > 0 {
		if s.Store.Box == nil {
			return apierror.New(503, "payload_unavailable", "Payload unavailable", "Audit encryption unavailable")
		}
		content, err = s.Store.Box.OpenBound(cipher, id, "audit.payload")
		if err != nil {
			return apierror.New(503, "payload_unavailable", "Payload unavailable", "Audit payload could not be decrypted")
		}
	} else if state == "captured" || state == "redacted" {
		state = "expired"
	}
	return writeJSON(w, 200, map[string]any{"state": state, "content": content})
}

func payloadNotFound() error {
	return apierror.New(404, "not_found", "Not found", "Audit event not found")
}
