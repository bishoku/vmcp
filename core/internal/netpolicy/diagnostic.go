package netpolicy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"syscall"
)

var (
	ErrHostNotAllowed        = errors.New("egress host is not allowed")
	ErrDestinationProhibited = errors.New("egress destination is prohibited")
	ErrPrivateDestination    = errors.New("private or loopback egress requires an allowed CIDR")
	ErrNoAddresses           = errors.New("egress destination has no addresses")
	ErrUnsupportedTransport  = errors.New("egress policy requires a standard HTTP transport")
	ErrInvalidPolicy         = errors.New("invalid egress policy configuration")
)

// Diagnostic returns allowlisted operator messages, never err.Error(). URL errors
// can contain query credentials, paths, header values or peer-controlled text.
func Diagnostic(err error) (reason, detail string) {
	for _, entry := range []struct {
		err            error
		reason, detail string
	}{
		{ErrHostNotAllowed, "egress_host_not_allowed", "Destination host is missing from VMCP_EGRESS_ALLOW_HOSTS."},
		{ErrPrivateDestination, "egress_private_address_not_allowed", "Private or loopback destination requires an explicit VMCP_EGRESS_ALLOW_CIDRS entry."},
		{ErrDestinationProhibited, "egress_destination_prohibited", "Destination address is prohibited by the egress policy."},
		{ErrNoAddresses, "dns_no_addresses", "DNS returned no destination addresses."},
		{ErrInvalidPolicy, "egress_invalid_configuration", "Check VMCP_EGRESS_ALLOW_HOSTS and VMCP_EGRESS_ALLOW_CIDRS syntax."},
		{ErrUnsupportedTransport, "egress_unsupported_transport", "Egress enforcement requires a standard HTTP transport."},
		{context.Canceled, "request_canceled", "The request context was canceled."},
		{context.DeadlineExceeded, "request_timeout", "The request deadline expired; the operation may have completed."},
		{syscall.ECONNREFUSED, "connection_refused", "The destination refused the TCP connection."},
		{syscall.ECONNRESET, "connection_reset", "The peer reset the connection; the operation may have completed."},
		{syscall.ENETUNREACH, "network_unreachable", "No network route to the destination."},
		{syscall.EHOSTUNREACH, "host_unreachable", "The destination host is unreachable."},
		{io.EOF, "connection_closed", "The peer closed the connection before a complete response."},
		{io.ErrUnexpectedEOF, "incomplete_response", "The peer returned an incomplete response."},
	} {
		if errors.Is(err, entry.err) {
			return entry.reason, entry.detail
		}
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		if dns.IsTimeout {
			return "dns_timeout", "DNS resolution timed out."
		}
		return "dns_failure", "DNS resolution failed."
	}
	var unknownCA x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var certificate x509.CertificateInvalidError
	var verification *tls.CertificateVerificationError
	if errors.As(err, &unknownCA) || errors.As(err, &hostname) || errors.As(err, &certificate) || errors.As(err, &verification) {
		return "tls_certificate_verification_failed", "TLS certificate verification failed; check trust, hostname and certificate validity."
	}
	var record tls.RecordHeaderError
	if errors.As(err, &record) {
		return "tls_protocol_error", "The destination did not provide a valid TLS response."
	}
	var network net.Error
	if errors.As(err, &network) && network.Timeout() {
		return "network_timeout", "A network operation timed out; the operation may have completed."
	}
	return "transport_error", "Transport failed; no safe structured cause is available."
}
