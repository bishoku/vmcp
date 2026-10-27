package omni

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sync"
	"syscall"
	"time"
)

// Only structured, non-payload diagnostics reach logs. Error() remains safe for clients.
type embeddingError struct {
	reason string
	attrs  []slog.Attr
}

func (e *embeddingError) Error() string { return "model request failed: " + e.reason }
func (e *embeddingError) LogValue() slog.Value {
	return slog.GroupValue(append([]slog.Attr{slog.String("reason", e.reason)}, e.attrs...)...)
}
func (e *HTTPEmbedder) failure(reason string, attrs ...slog.Attr) error {
	u, _ := url.Parse(e.Endpoint)
	// Do not log path/query: they may contain provider credentials.
	attrs = append(attrs, slog.String("provider_origin", u.Scheme+"://"+u.Host))
	if transport, ok := e.Client.Transport.(*http.Transport); ok && transport.TLSClientConfig != nil {
		attrs = append(attrs, slog.Bool("tls_skip_verify", transport.TLSClientConfig.InsecureSkipVerify))
	}
	return &embeddingError{reason: reason, attrs: attrs}
}
func transportReason(err error) string {
	var unknown x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	var tlsVerify *tls.CertificateVerificationError
	var dns *net.DNSError
	var record tls.RecordHeaderError
	switch {
	case errors.Is(err, context.Canceled):
		return "request_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.As(err, &unknown):
		return "tls_unknown_authority"
	case errors.As(err, &hostname):
		return "tls_hostname_mismatch"
	case errors.As(err, &invalid):
		return "tls_invalid_certificate"
	case errors.As(err, &tlsVerify):
		return "tls_certificate_verification"
	case errors.As(err, &record):
		return "tls_invalid_record"
	case errors.As(err, &dns):
		return "dns_lookup_failed"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection_refused"
	case errors.Is(err, syscall.ECONNRESET):
		return "connection_reset"
	}
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		return "timeout"
	}
	return "transport_error"
}

// First failure is immediate; repeated failures are summarized at most every 30s
// per operation. Separate state keeps search traffic from hiding refresh failures.
type failureLog struct {
	mu         sync.Mutex
	last       time.Time
	reason     string
	suppressed int
	failed     bool
}

func (l *failureLog) report(operation string, err error, attrs ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err == nil {
		if l.failed {
			slog.Info("omni dependency recovered", append([]any{"operation", operation, "suppressed_failures", l.suppressed}, attrs...)...)
		}
		l.failed = false
		l.suppressed = 0
		l.reason = ""
		return
	}
	var detail *embeddingError
	safe := any("dependency_error")
	reason := "dependency_error"
	if errors.As(err, &detail) {
		safe = detail
		reason = fmt.Sprint(detail.LogValue())
	} else {
		var h *HTTPError
		if errors.As(err, &h) {
			safe = h.Error()
			reason = h.Error()
		}
	}
	if l.failed && reason == l.reason && time.Since(l.last) < 30*time.Second {
		l.suppressed++
		return
	}
	slog.Warn("omni dependency failed", append([]any{"operation", operation, "error", safe, "suppressed_failures", l.suppressed}, attrs...)...)
	l.failed = true
	l.reason = reason
	l.last = time.Now()
	l.suppressed = 0
}
