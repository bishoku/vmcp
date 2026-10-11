package controlplane

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/store"
)

var errAuditResponseLimit = errors.New("audited response exceeds buffer limit")

const maxAuditResponseBytes = 8 << 20

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
	e := audit.Event{OperationID: id, RequestID: id, TraceID: audit.TraceID(r.Header.Get("traceparent")), ActorType: p.Via, ActorID: p.ID, Subject: p.Subject, Issuer: p.Issuer, Method: r.Method, Action: r.Pattern, Target: target, VirtualServer: ""}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip, err := netip.ParseAddr(host); err == nil {
		e.ClientIP = ip.Unmap().String()
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
