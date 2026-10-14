// Package controlplane implements the configuration REST API (see api/openapi.yaml).
package controlplane

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/oidc"
	"github.com/bishoku/vmcp/core/internal/store"
)

const (
	maxBodyBytes = 1 << 20
	defaultLimit = 50
	maxLimit     = 200
)

// Server is the control plane HTTP API.
type Server struct {
	Store          *store.Store
	AdminToken     string
	Version        string
	Log            *slog.Logger
	TrustedProxies []netip.Prefix
	// OIDC enables browser sign-in; nil means bearer tokens only.
	OIDC      *oidc.Provider
	auditOnce sync.Once
	auditor   *audit.Durable
}

type handlerFunc func(w http.ResponseWriter, r *http.Request) error

// Handler returns the full HTTP handler (API plus health probes).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Store.Pool.Ping(r.Context()); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})

	api := func(pattern string, h handlerFunc) { mux.Handle(pattern, s.wrap(h)) }
	mux.HandleFunc("GET /auth/config", s.authConfig)
	mux.HandleFunc("GET /auth/login", func(w http.ResponseWriter, r *http.Request) {
		if s.OIDC == nil {
			http.NotFound(w, r)
			return
		}
		s.OIDC.Login(w, r)
	})
	mux.HandleFunc("GET /auth/callback", func(w http.ResponseWriter, r *http.Request) {
		if s.OIDC == nil {
			http.NotFound(w, r)
			return
		}
		rec := &statusCapture{ResponseWriter: w, status: http.StatusOK}
		s.OIDC.Callback(rec, r)
		status := "ok"
		reason := ""
		if rec.status >= 400 {
			status = "denied"
			reason = "oidc callback rejected"
		}
		s.auditRecorder().Record(audit.Event{
			TraceID:   audit.TraceID(r.Header.Get("traceparent")),
			ActorType: "oidc",
			Issuer:    s.OIDC.Issuer(),
			ClientIP:  s.clientIPString(r),
			Method:    r.Method,
			Action:    "GET /auth/callback",
			Target:    "GET /auth/callback",
			Status:    status,
			Reason:    reason,
		})
	})
	mux.HandleFunc("POST /auth/logout", func(w http.ResponseWriter, r *http.Request) {
		var sub, iss string
		if s.OIDC != nil {
			if sess, ok := s.OIDC.Session(r); ok {
				sub = sess.Subject
				iss = s.OIDC.Issuer()
			}
			if !s.OIDC.Logout(w, r) {
				return
			}
			s.auditRecorder().Record(audit.Event{
				TraceID:   audit.TraceID(r.Header.Get("traceparent")),
				ActorType: "oidc",
				ActorID:   sub,
				Subject:   sub,
				Issuer:    iss,
				ClientIP:  s.clientIPString(r),
				Method:    r.Method,
				Action:    "POST /auth/logout",
				Target:    "POST /auth/logout",
				Status:    "ok",
			})
			if u := s.OIDC.EndSessionURL(); u != "" {
				_ = writeJSON(w, http.StatusOK, map[string]string{"endSessionUrl": u})
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})

	api("GET /api/v1/openapi.json", s.getOpenAPI)
	api("GET /api/v1/openapi.yaml", s.getOpenAPI)
	api("GET /api/v1/info", s.getInfo)
	api("GET /api/v1/me", s.getMe)

	api("GET /api/v1/upstreams", s.listUpstreams)
	api("POST /api/v1/upstreams", s.createUpstream)
	api("GET /api/v1/upstreams/{slug}", s.getUpstream)
	api("PUT /api/v1/upstreams/{slug}", s.putUpstream)
	api("PATCH /api/v1/upstreams/{slug}", s.patchUpstream)
	api("DELETE /api/v1/upstreams/{slug}", s.deleteUpstream)
	api("GET /api/v1/upstreams/{slug}/tools", s.listUpstreamTools)
	api("GET /api/v1/upstreams/{slug}/catalog", s.listUpstreamCatalog)
	api("POST /api/v1/upstreams/{slug}/refresh-tools", s.refreshTools)
	api("POST /api/v1/connection-tests", s.connectionTest)
	api("POST /api/v1/upstreams/{slug}/connection-test", s.storedConnectionTest)

	api("GET /api/v1/virtual-servers", s.listVirtualServers)
	api("POST /api/v1/virtual-servers", s.createVirtualServer)
	api("GET /api/v1/virtual-servers/{slug}", s.getVirtualServer)
	api("PUT /api/v1/virtual-servers/{slug}", s.putVirtualServer)
	api("PATCH /api/v1/virtual-servers/{slug}", s.patchVirtualServer)
	api("DELETE /api/v1/virtual-servers/{slug}", s.deleteVirtualServer)

	api("GET /api/v1/agent-keys", s.listAgentKeys)
	api("POST /api/v1/agent-keys", s.createAgentKey)
	api("GET /api/v1/agent-keys/{slug}", s.getAgentKey)
	api("PATCH /api/v1/agent-keys/{slug}", s.patchAgentKey)
	api("DELETE /api/v1/agent-keys/{slug}", s.deleteAgentKey)
	api("POST /api/v1/agent-keys/{slug}/rotate", s.rotateAgentKey)
	api("POST /api/v1/agent-keys/{slug}/revoke", s.revokeAgentKey)

	api("GET /api/v1/identity-bindings", s.listBindings)
	api("POST /api/v1/identity-bindings", s.createBinding)
	api("POST /api/v1/identity-bindings/evaluate", s.evaluateBindings)
	api("GET /api/v1/identity-bindings/{slug}", s.getBinding)
	api("PUT /api/v1/identity-bindings/{slug}", s.putBinding)
	api("PATCH /api/v1/identity-bindings/{slug}", s.patchBinding)
	api("DELETE /api/v1/identity-bindings/{slug}", s.deleteBinding)

	api("GET /api/v1/admin-tokens", s.listAdminTokens)
	api("POST /api/v1/admin-tokens", s.createAdminToken)
	api("GET /api/v1/admin-tokens/{slug}", s.getAdminToken)
	api("PATCH /api/v1/admin-tokens/{slug}", s.patchAdminToken)
	api("DELETE /api/v1/admin-tokens/{slug}", s.deleteAdminToken)
	api("POST /api/v1/admin-tokens/{slug}/rotate", s.rotateAdminToken)
	api("POST /api/v1/admin-tokens/{slug}/revoke", s.revokeAdminToken)

	api("POST /api/v1/revocations/jwts", s.revokeJWT)
	api("POST /api/v1/revocations/sessions", s.revokeSessions)

	api("GET /api/v1/audit-events", s.listAuditEvents)
	api("GET /api/v1/audit-status", s.auditStatus)

	api("GET /api/v1/export", s.exportConfig)
	api("POST /api/v1/apply", s.applyConfig)
	return mux
}

// principal is the authenticated caller.
type principal struct {
	Name    string `json:"name"`
	ID      string `json:"id,omitempty"`
	Subject string `json:"subject,omitempty"`
	Issuer  string `json:"issuer,omitempty"`
	Role    string `json:"role"`
	// Via is "bootstrap-token", "admin-token" or "oidc".
	Via string `json:"via"`
}

// wrap adds authentication, role checks and problem+json error rendering.
func (s *Server) wrap(h handlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := s.authorize(r)
		if err == nil && p == nil {
			err = apierror.Unauthorized()
		}
		if err == nil {
			err = permit(p.Role, r)
		}
		if err != nil {
			s.recordDenial(r, p, err)
			apierror.Write(w, r, err, s.Log)
			return
		}
		if err := s.audited(h, w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)), p); err != nil {
			apierror.Write(w, r, err, s.Log)
		}
	})
}

type principalKey struct{}

// permit enforces roles: viewers read; editors change configuration but not admin tokens or revocations;
// admins do everything.
func permit(role string, r *http.Request) error {
	safe := r.Method == http.MethodGet || r.Method == http.MethodHead
	switch role {
	case oidc.RoleAdmin:
		return nil
	case oidc.RoleEditor:
		if safe || (!strings.HasPrefix(r.URL.Path, "/api/v1/admin-tokens") && !strings.HasPrefix(r.URL.Path, "/api/v1/revocations")) {
			return nil
		}
		return apierror.Forbidden("editors cannot manage admin tokens or identity revocations")
	}
	if safe {
		return nil
	}
	return apierror.Forbidden("your role is read-only")
}

// authorize identifies the caller, or returns nil when credentials are missing or invalid.
// The static bootstrap token is always a full admin; named tokens come from the database;
// browser sessions come from OIDC.
func (s *Server) authorize(r *http.Request) (*principal, error) {
	if tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && tok != "" {
		// Hash first so the comparison time does not depend on the token length.
		a, b := sha256.Sum256([]byte(tok)), sha256.Sum256([]byte(s.AdminToken))
		if s.AdminToken != "" && subtle.ConstantTimeCompare(a[:], b[:]) == 1 {
			return &principal{Name: "bootstrap token", ID: "bootstrap", Role: oidc.RoleAdmin, Via: "bootstrap-token"}, nil
		}
		if !strings.HasPrefix(tok, "vmcpa_") {
			return nil, nil
		}
		scope, id, slug, ok, err := s.Store.LookupAdminIdentity(r.Context(), tok)
		if err != nil || !ok {
			return nil, err
		}
		role := oidc.RoleViewer
		if scope == store.ScopeAdmin {
			role = oidc.RoleAdmin
		}
		return &principal{Name: slug, ID: id, Role: role, Via: "admin-token"}, nil
	}
	if s.OIDC == nil {
		return nil, nil
	}
	sess, ok := s.OIDC.Session(r)
	if !ok {
		return nil, nil
	}
	p := &principal{Name: sess.Name, ID: sess.Subject, Subject: sess.Subject, Issuer: s.OIDC.Issuer(), Role: sess.Role, Via: "oidc"}
	// Cookies are sent automatically by browsers; demand a header a cross-site form cannot set.
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("X-VMCP-CSRF") == "" {
		return p, apierror.Forbidden("missing X-VMCP-CSRF header")
	}
	return p, nil
}

func (s *Server) getMe(w http.ResponseWriter, r *http.Request) error {
	return writeJSON(w, http.StatusOK, r.Context().Value(principalKey{}))
}

func (s *Server) authConfig(w http.ResponseWriter, _ *http.Request) {
	_ = writeJSON(w, http.StatusOK, map[string]any{"oidc": s.OIDC != nil})
}

func writeJSON(w http.ResponseWriter, status int, v any) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
	return nil
}

// decode reads a strict JSON body: unknown fields and trailing data are rejected.
func decode(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return apierror.Validation([]apierror.FieldError{{Code: apierror.CodeValidationFailed, Message: "invalid JSON body: " + err.Error()}})
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return apierror.Validation([]apierror.FieldError{{Code: apierror.CodeValidationFailed, Message: "unexpected data after the JSON body"}})
	}
	return nil
}

func setETag(w http.ResponseWriter, version int64) {
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(version, 10)))
}

// parseIfMatch returns nil when the header is absent or "*".
func parseIfMatch(r *http.Request) (*int64, error) {
	h := strings.TrimSpace(r.Header.Get("If-Match"))
	if h == "" || h == "*" {
		return nil, nil
	}
	v, err := strconv.ParseInt(strings.Trim(h, `"`), 10, 64)
	if err != nil {
		return nil, apierror.PreconditionFailed()
	}
	return &v, nil
}

func (s *Server) setRevision(w http.ResponseWriter, r *http.Request) {
	if rev, err := store.Revision(r.Context(), s.Store.Pool); err == nil {
		w.Header().Set("X-VMCP-Config-Revision", strconv.FormatInt(rev, 10))
	}
}

func pageParams(r *http.Request) (limit int, after string, enabled *bool, err error) {
	limit = defaultLimit
	q := r.URL.Query()
	var errs []apierror.FieldError
	if v := q.Get("limit"); v != "" {
		n, perr := strconv.Atoi(v)
		if perr != nil || n < 1 || n > maxLimit {
			errs = append(errs, apierror.FieldError{Field: "limit", Code: apierror.CodeValidationFailed, Message: fmt.Sprintf("must be an integer between 1 and %d", maxLimit)})
		} else {
			limit = n
		}
	}
	if v := q.Get("cursor"); v != "" {
		b, perr := base64.RawURLEncoding.DecodeString(v)
		if perr != nil {
			errs = append(errs, apierror.FieldError{Field: "cursor", Code: apierror.CodeValidationFailed, Message: "invalid cursor"})
		}
		after = string(b)
	}
	if v := q.Get("enabled"); v != "" {
		b, perr := strconv.ParseBool(v)
		if perr != nil {
			errs = append(errs, apierror.FieldError{Field: "enabled", Code: apierror.CodeValidationFailed, Message: "must be true or false"})
		} else {
			enabled = &b
		}
	}
	if len(errs) > 0 {
		err = apierror.Validation(errs)
	}
	return limit, after, enabled, err
}

func cursorFor(slug string) *string {
	c := base64.RawURLEncoding.EncodeToString([]byte(slug))
	return &c
}

func queryBool(r *http.Request, name string, def bool) bool {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func (s *Server) getInfo(w http.ResponseWriter, r *http.Request) error {
	rev, err := store.Revision(r.Context(), s.Store.Pool)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"version": s.Version, "configRevision": rev, "tenantId": store.TenantID()})
}
