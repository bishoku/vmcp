package controlplane

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/bishoku/vmcp/core/internal/agentauth"
	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/oidc"
	"github.com/bishoku/vmcp/core/internal/store"
)

var errAuditResponseLimit = errors.New("audited response exceeds buffer limit")

const maxAuditResponseBytes = 8 << 20

type statusCapture struct {
	http.ResponseWriter
	status int
}

func (w *statusCapture) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

type auditResponse struct {
	forward    http.ResponseWriter
	totalBytes int
	header     http.Header
	body       bytes.Buffer
	status     int
}

func (w *auditResponse) Header() http.Header {
	if w.forward != nil {
		return w.forward.Header()
	}
	return w.header
}
func (w *auditResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		if w.forward != nil {
			w.forward.WriteHeader(status)
		}
	}
}
func (w *auditResponse) Write(p []byte) (int, error) {
	w.totalBytes += len(p)
	if w.status == 0 {
		w.status = 200
	}
	if w.forward != nil {
		n, err := w.forward.Write(p)
		if w.body.Len() < 2<<20+1 {
			_, _ = w.body.Write(p[:min(n, (2<<20+1)-w.body.Len())])
		}
		return n, err
	}
	if w.body.Len()+len(p) > maxAuditResponseBytes {
		return 0, errAuditResponseLimit
	}
	return w.body.Write(p)
}

func anyContainsPrefix(ps []netip.Prefix, ip netip.Addr) bool {
	for _, p := range ps {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

func (s *Server) clientIPString(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return ""
	}
	peer = peer.Unmap()
	if !anyContainsPrefix(s.TrustedProxies, peer) {
		return peer.String()
	}
	parts := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
		if err != nil {
			return peer.String()
		}
		a = a.Unmap()
		if !anyContainsPrefix(s.TrustedProxies, a) {
			return a.String()
		}
		peer = a
	}
	return peer.String()
}

func (s *Server) recordDenial(r *http.Request, p *principal, err error) {
	var prob *apierror.Error
	if !errors.As(err, &prob) || (prob.Status != http.StatusUnauthorized && prob.Status != http.StatusForbidden) {
		return
	}
	target := r.Pattern
	if target == "" {
		target = r.Method + " " + r.URL.Path
	} else if slug := r.PathValue("slug"); store.ValidSlug(slug) {
		target += ":" + slug
	}
	action := r.Pattern
	if action == "" {
		action = r.Method + " " + r.URL.Path
	}
	ev := audit.Event{
		TraceID:  audit.TraceID(r.Header.Get("traceparent")),
		ClientIP: s.clientIPString(r),
		Method:   r.Method,
		Action:   action,
		Target:   target,
		Status:   "denied",
		Reason:   prob.Detail,
	}
	if p != nil {
		ev.ActorType = p.Via
		ev.ActorID = p.ID
		ev.Subject = p.Subject
		ev.Issuer = p.Issuer
	}
	s.auditRecorder().Record(ev)
}

func (s *Server) audited(h handlerFunc, w http.ResponseWriter, r *http.Request, p *principal) error {
	if strings.HasPrefix(r.URL.Path, "/api/v1/openapi.") {
		return h(w, r)
	}
	if s.Store == nil {
		if r.Method == "GET" || r.Method == "HEAD" {
			return h(w, r)
		}
		return auditUnavailable()
	}
	recorder := s.auditRecorder()
	settings, settingsErr := store.LoadAuditSettingsIfChanged(r.Context(), s.Store.Pool, recorder.Settings())
	if settingsErr != nil {
		return auditUnavailable()
	}
	recorder.SetSettings(settings)
	sensitive := r.Method != "GET" && r.Method != "HEAD" || r.URL.Path == "/api/v1/export" || strings.HasPrefix(r.URL.Path, "/api/v1/upstreams")
	if c := settings.Scopes["reads"]; c.Request || c.Response {
		sensitive = true
	}
	if strings.HasSuffix(r.URL.Path, "/payload") {
		sensitive = true
	}
	if !sensitive {
		return h(w, r)
	}
	id, err := audit.NewID()
	if err != nil {
		return auditUnavailable()
	}
	// Only matched route patterns and validated path identifiers enter audit metadata.
	target := r.Pattern
	if slug := r.PathValue("slug"); store.ValidSlug(slug) {
		target += ":" + slug
	}
	if strings.HasSuffix(r.URL.Path, "/payload") && audit.ValidID(r.PathValue("event")) {
		target += ":" + r.PathValue("event")
	}
	e := audit.Event{
		OperationID:   id,
		RequestID:     id,
		TraceID:       audit.TraceID(r.Header.Get("traceparent")),
		ActorType:     p.Via,
		ActorID:       p.ID,
		Subject:       p.Subject,
		Issuer:        p.Issuer,
		ClientIP:      s.clientIPString(r),
		Method:        r.Method,
		Action:        r.Pattern,
		Target:        target,
		VirtualServer: "",
		Policy:        &settings, Scope: "control", Boundary: "control",
	}
	if r.Method == "GET" || r.Method == "HEAD" {
		e.Scope = "reads"
	}
	// Object context comes from matched routes, never arbitrary body data.
	if strings.HasPrefix(r.URL.Path, "/api/v1/upstreams/") {
		e.Upstream = r.PathValue("slug")
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/virtual-servers/") {
		e.VirtualServer = r.PathValue("slug")
	}
	excluded := r.URL.Path == "/api/v1/audit-preview" || strings.HasPrefix(r.URL.Path, "/api/v1/audit-events") || r.URL.Path == "/api/v1/audit-settings"
	if excluded {
		e.Scope = ""
	}
	d := recorder
	w.Header().Set("X-Request-Id", id)
	// Admission must contain the request before the handler runs. Read only
	// when request capture is enabled, and preserve existing handler limits.
	if !excluded && settings.Scopes[e.Scope].Request && r.Body != nil {
		raw, readErr := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
		if readErr != nil {
			return apierror.New(400, "invalid_body", "Invalid body", "request could not be read")
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		e.RawPayload = raw
	}
	e, err = d.Admit(r.Context(), e)
	if err != nil {
		return auditUnavailable()
	}
	r = r.WithContext(audit.WithOperation(audit.WithFailureReporter(audit.WithTransaction(r.Context(), e), d.ReportWriteFailure), d, e))
	response := &auditResponse{header: make(http.Header)}
	if e.Mode == "relax" {
		response.forward = w
	}
	start := time.Now()
	err = h(response, r)
	e.Status = "ok"
	if err != nil {
		e.Status = "error"
		e.Reason = "administrative operation failed"
	}
	if err != nil {
		if response.forward == nil {
			response.body.Reset()
			response.header = make(http.Header)
			response.status = 0
			response.totalBytes = 0
		}
		apierror.Write(response, r, err, s.Log)
		err = nil
	}
	e.RawPayload = response.body.Bytes()
	e.RawPayloadBytes = response.totalBytes
	e.DurationMS = int(time.Since(start).Milliseconds())
	// Client cancellation cannot erase an outcome for an already committed mutation.
	completion, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), audit.WriteTimeout)
	defer cancel()
	if completionErr := d.Complete(completion, e); completionErr != nil {
		return auditUnavailable()
	}
	if err != nil {
		return err
	}
	if response.forward != nil {
		return nil
	}
	for key, values := range response.header {
		w.Header()[key] = values
	}
	status := response.status
	if status == 0 {
		status = 200
	}
	w.WriteHeader(status)
	_, err = w.Write(response.body.Bytes())
	return err
}

func auditUnavailable() error {
	return apierror.New(503, "audit_unavailable", "Audit unavailable", "audit persistence unavailable; operation may be incomplete")
}

func (s *Server) auditStatus(w http.ResponseWriter, r *http.Request) error {
	pending, err := audit.Pending(r.Context(), s.Store.Pool)
	if err != nil {
		return auditUnavailable()
	}
	return writeJSON(w, 200, map[string]any{"process": s.auditRecorder().Stats(), "durable": pending})
}

func (s *Server) auditRecorder() *audit.Runtime {
	s.auditOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		s.auditCancel = cancel
		s.auditor = audit.NewRuntime(ctx, s.Store.Pool, s.Store.Box, s.Log, "strict")
	})
	return s.auditor
}

func (s *Server) revokeJWT(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Token     string     `json:"token"`
		TokenHash string     `json:"tokenHash"`
		ExpiresAt *time.Time `json:"expiresAt"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	var hash []byte
	switch {
	case body.Token != "" && body.TokenHash != "":
		return apierror.Validation([]apierror.FieldError{{Field: "token", Code: apierror.CodeValidationFailed, Message: "provide either token or tokenHash, not both"}})
	case body.Token != "":
		sum := sha256.Sum256([]byte(body.Token))
		hash = sum[:]
	case body.TokenHash != "":
		decoded, err := hex.DecodeString(strings.TrimSpace(body.TokenHash))
		if err != nil || len(decoded) != 32 {
			return apierror.Validation([]apierror.FieldError{{Field: "tokenHash", Code: apierror.CodeValidationFailed, Message: "must be a 64-character hex SHA-256 digest"}})
		}
		hash = decoded
	default:
		return apierror.Validation([]apierror.FieldError{{Field: "tokenHash", Code: apierror.CodeValidationFailed, Message: "token or tokenHash is required"}})
	}
	expiresAt := time.Now().UTC().Add(24 * time.Hour)
	if body.ExpiresAt != nil {
		if !body.ExpiresAt.After(time.Now()) {
			return apierror.Validation([]apierror.FieldError{{Field: "expiresAt", Code: apierror.CodeValidationFailed, Message: "must be in the future"}})
		}
		expiresAt = body.ExpiresAt.UTC()
	}
	if err := (agentauth.PostgresRevocations{Pool: s.Store.Pool}).Revoke(r.Context(), hash, expiresAt); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) revokeSessions(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Subject string `json:"subject"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	sub := strings.TrimSpace(body.Subject)
	if sub == "" || len(sub) > 512 {
		return apierror.Validation([]apierror.FieldError{{Field: "subject", Code: apierror.CodeValidationFailed, Message: "subject is required"}})
	}
	if err := (oidc.PostgresSessions{Pool: s.Store.Pool}).RevokeSubject(r.Context(), sub); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) CloseAudit() {
	if s.auditCancel != nil {
		s.auditCancel()
		<-s.auditor.Done()
	}
}
