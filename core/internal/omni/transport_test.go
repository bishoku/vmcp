package omni

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestProviderRequiresEgressGrantAndAcceptsPrivateCA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"index":0,"embedding":[1]}]}`))
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VMCP_OMNI_EMBEDDING_CA_FILE", path)
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "")
	e, err := NewHTTPEmbedder(srv.URL, "model", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Embed(context.Background(), []string{"query"}); err == nil {
		t.Fatal("private destination allowed without grant")
	}
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	e, err = NewHTTPEmbedder(srv.URL, "model", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Embed(context.Background(), []string{"query"}); err != nil {
		t.Fatal("trusted private CA failed", err)
	}
	if _, err = NewHTTPEmbedder(srv.URL, "model", "", true); err == nil {
		t.Fatal("CA with verification bypass")
	}
}
