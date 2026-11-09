package omni

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/bishoku/vmcp/core/internal/netpolicy"
)

func providerClient(caEnv string, insecure bool, timeout time.Duration) (*http.Client, error) {
	if _, err := netpolicy.FromEnv(); err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: insecure} // Explicit legacy operator opt-in.
	if path := os.Getenv(caEnv); path != "" {
		if insecure {
			return nil, errors.New("private CA and insecure TLS cannot be combined")
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, errors.New("provider CA file is unavailable")
		}
		defer file.Close()
		pem, err := io.ReadAll(io.LimitReader(file, 65537))
		if err != nil || len(pem) > 65536 {
			return nil, errors.New("provider CA file is unavailable or oversized")
		}
		roots, err := x509.SystemCertPool()
		if err != nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(pem) {
			return nil, errors.New("provider CA file has no certificates")
		}
		transport.TLSClientConfig.RootCAs = roots
	}
	return netpolicy.DedicatedClient(&http.Client{Transport: transport, Timeout: timeout}), nil
}
