package auditexport

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
)

func TestTLSValidation(t *testing.T) {
	for _, k := range []string{"VMCP_AUDIT_EXPORT_TLS_CA_FILE", "VMCP_AUDIT_EXPORT_TLS_CERT_FILE", "VMCP_AUDIT_EXPORT_TLS_KEY_FILE"} {
		t.Setenv(k, "")
	}
	cfg, err := TLSConfig()
	if err != nil || cfg.InsecureSkipVerify || cfg.MinVersion < tls.VersionTLS12 {
		t.Fatal("unsafe TLS defaults", err)
	}
	path := filepath.Join(t.TempDir(), "ca.pem")
	os.WriteFile(path, []byte("invalid"), 0600)
	t.Setenv("VMCP_AUDIT_EXPORT_TLS_CA_FILE", path)
	if _, err = TLSConfig(); err == nil {
		t.Fatal("invalid CA accepted")
	}
	t.Setenv("VMCP_AUDIT_EXPORT_TLS_CA_FILE", "")
	t.Setenv("VMCP_AUDIT_EXPORT_TLS_KEY_FILE", path)
	if _, err = TLSConfig(); err == nil {
		t.Fatal("unpaired client key accepted")
	}
}
