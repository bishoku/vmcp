package netpolicy

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"syscall"
	"testing"
)

func TestDiagnosticPreservesSafeReasonsWithoutLeakingPeerText(t *testing.T) {
	for _, test := range []struct {
		cause error
		want  string
	}{
		{ErrHostNotAllowed, "egress_host_not_allowed"},
		{ErrPrivateDestination, "egress_private_address_not_allowed"},
		{ErrDestinationProhibited, "egress_destination_prohibited"},
		{fmt.Errorf("%w: secret", ErrInvalidPolicy), "egress_invalid_configuration"},
		{context.DeadlineExceeded, "request_timeout"},
		{context.Canceled, "request_canceled"},
		{&net.DNSError{Name: "secret", Err: "secret"}, "dns_failure"},
		{&net.DNSError{Name: "secret", IsTimeout: true}, "dns_timeout"},
		{&net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, "connection_refused"},
		{syscall.ECONNRESET, "connection_reset"},
		{x509.HostnameError{Host: "secret"}, "tls_certificate_verification_failed"},
		{io.EOF, "connection_closed"},
		{errors.New("Authorization: Bearer secret"), "transport_error"},
	} {
		t.Run(test.want, func(t *testing.T) {
			err := &url.Error{Op: "Get", URL: "https://secret.example/secret?token=secret", Err: test.cause}
			reason, detail := Diagnostic(fmt.Errorf("secret: %w", err))
			if reason != test.want || strings.Contains(reason+detail, "secret") {
				t.Fatalf("unsafe or incorrect diagnostic: %q %q", reason, detail)
			}
		})
	}
}
