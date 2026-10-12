package auditexport

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
)

// TLSConfig always verifies server identity. Secret files are read only at startup.
func TLSConfig() (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if path := os.Getenv("VMCP_AUDIT_EXPORT_TLS_CA_FILE"); path != "" {
		pem, err := os.ReadFile(path)
		if err != nil {
			return nil, errors.New("cannot read audit broker CA")
		}
		roots, err := x509.SystemCertPool()
		if err != nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(pem) {
			return nil, errors.New("invalid audit broker CA")
		}
		cfg.RootCAs = roots
	}
	cert, key := os.Getenv("VMCP_AUDIT_EXPORT_TLS_CERT_FILE"), os.Getenv("VMCP_AUDIT_EXPORT_TLS_KEY_FILE")
	if (cert == "") != (key == "") {
		return nil, errors.New("both audit client certificate and key are required")
	}
	if cert != "" {
		pair, err := tls.LoadX509KeyPair(cert, key)
		if err != nil {
			return nil, errors.New("invalid audit client certificate/key")
		}
		cfg.Certificates = []tls.Certificate{pair}
	}
	return cfg, nil
}
