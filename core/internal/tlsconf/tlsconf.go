// Package tlsconf holds per-upstream TLS settings (custom CA, SNI override, mTLS).
// There is deliberately no option to skip certificate verification.
package tlsconf

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	MaxPEMBytes = 64 << 10
	MaxCerts    = 10
)

// Settings are the decrypted TLS settings of one upstream.
type Settings struct {
	CAPEM         string
	ServerName    string
	ClientCertPEM string
	ClientKeyPEM  string
}

// IsZero reports whether the default system TLS behaviour applies.
func (s Settings) IsZero() bool { return s == Settings{} }

// CertInfo describes a parsed certificate for display.
type CertInfo struct {
	Subject  string    `json:"subject"`
	NotAfter time.Time `json:"notAfter"`
}

// Info is the read model returned by the API. The private key is never included.
type Info struct {
	CAPEM         string     `json:"caPem"`
	ServerName    string     `json:"serverName"`
	ClientCertPEM string     `json:"clientCertPem"`
	HasClientKey  bool       `json:"hasClientKey"`
	CACerts       []CertInfo `json:"caCerts"`
	ClientCert    *CertInfo  `json:"clientCert"`
}

var hostRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$`)

func parseCerts(p string) ([]*x509.Certificate, bool) {
	var out []*x509.Certificate
	rest := []byte(p)
	for {
		var b *pem.Block
		b, rest = pem.Decode(rest)
		if b == nil {
			break
		}
		if b.Type != "CERTIFICATE" {
			return nil, false
		}
		c, err := x509.ParseCertificate(b.Bytes)
		if err != nil {
			return nil, false
		}
		out = append(out, c)
	}
	if len(strings.TrimSpace(string(rest))) > 0 {
		return nil, false
	}
	return out, len(out) > 0
}

// FieldError is a validation problem on one TLS field (name is the JSON field).
type FieldError struct{ Field, Message string }

// Validate checks the settings and returns problems keyed by field name.
func (s Settings) Validate() []FieldError {
	var errs []FieldError
	if s.CAPEM != "" {
		cs, ok := parseCerts(s.CAPEM)
		switch {
		case len(s.CAPEM) > MaxPEMBytes:
			errs = append(errs, FieldError{"caPem", "must be at most 64 KiB"})
		case !ok:
			errs = append(errs, FieldError{"caPem", "must be PEM with at least one valid CERTIFICATE block and nothing else"})
		case len(cs) > MaxCerts:
			errs = append(errs, FieldError{"caPem", "must contain at most 10 certificates"})
		}
	}
	if s.ServerName != "" && (len(s.ServerName) > 253 || !hostRe.MatchString(s.ServerName)) {
		errs = append(errs, FieldError{"serverName", "must be a valid host name"})
	}
	switch {
	case s.ClientCertPEM == "" && s.ClientKeyPEM == "":
	case s.ClientCertPEM == "" || s.ClientKeyPEM == "":
		errs = append(errs, FieldError{"clientCertPem", "client certificate and key must be provided together"})
	case len(s.ClientCertPEM) > MaxPEMBytes || len(s.ClientKeyPEM) > MaxPEMBytes:
		errs = append(errs, FieldError{"clientCertPem", "must be at most 64 KiB"})
	default:
		if _, err := tls.X509KeyPair([]byte(s.ClientCertPEM), []byte(s.ClientKeyPEM)); err != nil {
			errs = append(errs, FieldError{"clientCertPem", "client certificate and key are not a valid pair"})
		}
	}
	return errs
}

// Info builds the API read model.
func (s Settings) Info() Info {
	i := Info{CAPEM: s.CAPEM, ServerName: s.ServerName, ClientCertPEM: s.ClientCertPEM,
		HasClientKey: s.ClientKeyPEM != "", CACerts: []CertInfo{}}
	if cs, ok := parseCerts(s.CAPEM); ok {
		for _, c := range cs {
			i.CACerts = append(i.CACerts, CertInfo{c.Subject.String(), c.NotAfter})
		}
	}
	if cs, ok := parseCerts(s.ClientCertPEM); ok {
		i.ClientCert = &CertInfo{cs[0].Subject.String(), cs[0].NotAfter}
	}
	return i
}

var (
	mu      sync.Mutex
	clients = map[[32]byte]*http.Client{}
)

// Client returns an HTTP client for the settings, or nil for the default behaviour.
// Clients are cached by settings so connection pools are reused.
func (s Settings) Client() (*http.Client, error) {
	if s.IsZero() {
		return nil, nil
	}
	key := sha256.Sum256([]byte(s.CAPEM + "\x00" + s.ServerName + "\x00" + s.ClientCertPEM + "\x00" + s.ClientKeyPEM))
	mu.Lock()
	defer mu.Unlock()
	if c, ok := clients[key]; ok {
		return c, nil
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: s.ServerName}
	if s.CAPEM != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(s.CAPEM)) {
			return nil, errBadCA
		}
		cfg.RootCAs = pool
	}
	if s.ClientCertPEM != "" {
		cert, err := tls.X509KeyPair([]byte(s.ClientCertPEM), []byte(s.ClientKeyPEM))
		if err != nil {
			return nil, err
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConns = 2048
	tr.MaxIdleConnsPerHost = 128
	tr.IdleConnTimeout = 90 * time.Second
	tr.TLSClientConfig = cfg
	c := &http.Client{Transport: tr}
	clients[key] = c
	return c, nil
}

type tlsErr string

func (e tlsErr) Error() string { return string(e) }

const errBadCA = tlsErr("tls: CA bundle contains no usable certificate")
