package controlplane

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
	header http.Header
	body   bytes.Buffer
	status int
}

func (w *auditResponse) Header() http.Header { return w.header }
func (w *auditResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *auditResponse) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
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
	sensitive := r.Method != "GET" && r.Method != "HEAD" || r.URL.Path == "/api/v1/export" || strings.HasPrefix(r.URL.Path, "/api/v1/upstreams")
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
	}
	d := s.auditRecorder()
	w.Header().Set("X-Request-Id", id)
	e, err = d.Admit(r.Context(), e)
	if err != nil {
		return auditUnavailable()
	}
	r = r.WithContext(audit.WithFailureReporter(audit.WithTransaction(r.Context(), e), d.ReportWriteFailure))
	response := &auditResponse{header: make(http.Header)}
	start := time.Now()
	err = h(response, r)
	e.Status = "ok"
	if err != nil {
		e.Status = "error"
		e.Reason = "administrative operation failed"
	}
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

func (s *Server) auditRecorder() *audit.Durable {
	s.auditOnce.Do(func() { s.auditor = audit.NewDurable(s.Store.Pool, s.Log) })
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

