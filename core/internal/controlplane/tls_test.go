package controlplane_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/fixture"
)

type pki struct {
	caPEM string
	ca    *x509.Certificate
	key   *ecdsa.PrivateKey
}

func newPKI(t *testing.T, cn string) *pki {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := x509.ParseCertificate(der)
	return &pki{caPEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), ca: ca, key: key}
}

// leaf issues a certificate and returns it as PEM cert, PEM key and tls.Certificate.
func (p *pki) leaf(t *testing.T, cn string, dns []string, ips []net.IP, client bool) (string, string, tls.Certificate) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	usage := x509.ExtKeyUsageServerAuth
	if client {
		usage = x509.ExtKeyUsageClientAuth
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage},
		DNSNames: dns, IPAddresses: ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, p.ca, &key.PublicKey, p.key)
	if err != nil {
		t.Fatal(err)
	}
	kd, _ := x509.MarshalECPrivateKey(key)
	cp := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	kp := string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kd}))
	c, err := tls.X509KeyPair([]byte(cp), []byte(kp))
	if err != nil {
		t.Fatal(err)
	}
	return cp, kp, c
}

func tlsServer(t *testing.T, cert tls.Certificate, clientCAs *x509.CertPool) string {
	t.Helper()
	srv := httptest.NewUnstartedServer(fixture.New(fixture.Options{}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	if clientCAs != nil {
		srv.TLS.ClientCAs = clientCAs
		srv.TLS.ClientAuth = tls.RequireAndVerifyClientCert
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv.URL + "/mcp"
}

func TestUpstreamTLS(t *testing.T) {
	a := setup(t)
	ca := newPKI(t, "Test CA")
	other := newPKI(t, "Other CA")
	_, _, srvCert := ca.leaf(t, "upstream", []string{"upstream.test"}, []net.IP{net.ParseIP("127.0.0.1")}, false)
	url := tlsServer(t, srvCert, nil)

	probe := func(tl map[string]any) bool {
		t.Helper()
		body := map[string]any{"url": url}
		if tl != nil {
			body["tls"] = tl
		}
		_, res := a.expect(200, "POST", "/api/v1/connection-tests", body)
		return res["ok"] == true
	}
	if probe(nil) {
		t.Fatal("self-signed upstream must be rejected without a trusted CA")
	}
	if !probe(map[string]any{"caPem": ca.caPEM}) {
		t.Fatal("upstream must be reachable with its CA")
	}
	if probe(map[string]any{"caPem": other.caPEM}) {
		t.Fatal("a different CA must not be trusted")
	}

	// Validation.
	for _, bad := range []map[string]any{
		{"caPem": "not a pem"},
		{"caPem": ca.caPEM, "serverName": "bad name!"},
		{"clientCertPem": ca.caPEM},
	} {
		a.expect(422, "POST", "/api/v1/connection-tests", map[string]any{"url": url, "tls": bad})
	}

	// Stored settings are used by refresh and shown without the key.
	a.expect(201, "POST", "/api/v1/upstreams", map[string]any{"slug": "tls1", "name": "tls1", "url": url, "tls": map[string]any{"caPem": ca.caPEM}})
	a.expect(200, "POST", "/api/v1/upstreams/tls1/refresh-tools", nil)
	_, up := a.expect(200, "GET", "/api/v1/upstreams/tls1", nil)
	info := up["tls"].(map[string]any)
	if info["caPem"] != ca.caPEM || len(info["caCerts"].([]any)) != 1 || info["hasClientKey"] != false {
		t.Fatalf("unexpected tls info: %v", info)
	}
	// PATCH without tls keeps it; clearing the CA breaks refresh.
	a.expect(200, "PATCH", "/api/v1/upstreams/tls1", map[string]any{"name": "renamed"})
	a.expect(200, "POST", "/api/v1/upstreams/tls1/refresh-tools", nil)
	a.expect(200, "PATCH", "/api/v1/upstreams/tls1", map[string]any{"tls": map[string]any{"caPem": ""}})
	a.expect(502, "POST", "/api/v1/upstreams/tls1/refresh-tools", nil)

	// Export carries the CA but apply round-trips it.
	a.expect(200, "PATCH", "/api/v1/upstreams/tls1", map[string]any{"tls": map[string]any{"caPem": ca.caPEM}})
	_, doc := a.expect(200, "GET", "/api/v1/export", nil)
	ups := doc["upstreams"].([]any)
	found := false
	for _, u := range ups {
		m := u.(map[string]any)
		if m["slug"] == "tls1" {
			found = true
			if m["tls"].(map[string]any)["caPem"] != ca.caPEM {
				t.Fatalf("export lacks caPem: %v", m)
			}
		}
	}
	if !found {
		t.Fatal("tls1 not exported")
	}
}

func TestUpstreamTLSServerNameAndMTLS(t *testing.T) {
	a := setup(t)
	ca := newPKI(t, "Test CA")
	// Certificate valid for the DNS name only; we dial the IP.
	_, _, srvCert := ca.leaf(t, "upstream", []string{"upstream.test"}, nil, false)
	pool := x509.NewCertPool()
	pool.AddCert(ca.ca)
	url := tlsServer(t, srvCert, pool)
	clientCert, clientKey, _ := ca.leaf(t, "agent", nil, nil, true)

	probe := func(tl map[string]any) bool {
		t.Helper()
		_, res := a.expect(200, "POST", "/api/v1/connection-tests", map[string]any{"url": url, "tls": tl})
		return res["ok"] == true
	}
	if probe(map[string]any{"caPem": ca.caPEM, "clientCertPem": clientCert, "clientKeyPem": clientKey}) {
		t.Fatal("IP dial must fail name verification without serverName")
	}
	if probe(map[string]any{"caPem": ca.caPEM, "serverName": "upstream.test"}) {
		t.Fatal("server requiring a client certificate must reject a client without one")
	}
	if !probe(map[string]any{"caPem": ca.caPEM, "serverName": "upstream.test", "clientCertPem": clientCert, "clientKeyPem": clientKey}) {
		t.Fatal("serverName override + mTLS should connect")
	}

	// The key is stored encrypted, never returned, and kept across a PATCH of other TLS fields.
	a.expect(201, "POST", "/api/v1/upstreams", map[string]any{"slug": "m1", "name": "m1", "url": url, "tls": map[string]any{
		"caPem": ca.caPEM, "serverName": "upstream.test", "clientCertPem": clientCert, "clientKeyPem": clientKey}})
	a.expect(200, "POST", "/api/v1/upstreams/m1/refresh-tools", nil)
	a.expect(200, "PATCH", "/api/v1/upstreams/m1", map[string]any{"tls": map[string]any{"serverName": "upstream.test"}})
	a.expect(200, "POST", "/api/v1/upstreams/m1/refresh-tools", nil)
	h, up := a.expect(200, "GET", "/api/v1/upstreams/m1", nil)
	_ = h
	info := up["tls"].(map[string]any)
	if info["hasClientKey"] != true || strings.Contains(toJSON(up), "PRIVATE KEY") {
		t.Fatalf("key handling wrong: %v", info)
	}
	_, doc := a.expect(200, "GET", "/api/v1/export", nil)
	if strings.Contains(toJSON(doc), "PRIVATE KEY") {
		t.Fatal("export leaks the client key")
	}
	var raw int
	if err := a.pool.QueryRow(context.Background(), `SELECT count(*) FROM upstreams WHERE slug='m1' AND encrypted_client_key IS NOT NULL AND position('PRIVATE'::bytea in encrypted_client_key)=0`).Scan(&raw); err != nil || raw != 1 {
		t.Fatalf("client key not stored encrypted (%d, %v)", raw, err)
	}
	// Clearing the certificate clears the key.
	a.expect(200, "PATCH", "/api/v1/upstreams/m1", map[string]any{"tls": map[string]any{"clientCertPem": ""}})
	_, up = a.expect(200, "GET", "/api/v1/upstreams/m1", nil)
	if up["tls"].(map[string]any)["hasClientKey"] != false {
		t.Fatal("key should be cleared with the certificate")
	}
}

func TestToolDescriptionOverrideDrift(t *testing.T) {
	a := setup(t)
	a.expect(201, "POST", "/api/v1/upstreams", a.upstream())
	a.expect(200, "POST", "/api/v1/upstreams/fx/refresh-tools", nil)
	sel := func(desc string) map[string]any {
		return map[string]any{"slug": "vs", "name": "VS", "tools": []map[string]string{{"upstream": "fx", "tool": "echo", "description": desc}}}
	}
	first := func() map[string]any {
		t.Helper()
		_, out := a.expect(200, "GET", "/api/v1/virtual-servers/vs", nil)
		return out["tools"].([]any)[0].(map[string]any)
	}
	a.expect(201, "POST", "/api/v1/virtual-servers", sel("Custom text"))
	tl := first()
	if tl["description"] != "Custom text" || tl["upstreamDescription"] == "" || tl["descriptionDrift"] != false {
		t.Fatalf("unexpected selection: %v", tl)
	}
	_, doc := a.expect(200, "GET", "/api/v1/export", nil)
	if !strings.Contains(toJSON(doc), "Custom text") {
		t.Fatal("export lacks the override")
	}

	// The upstream text changes after the override was written: drift is flagged.
	if _, err := a.pool.Exec(context.Background(), `UPDATE upstream_tools SET description = 'New upstream text' WHERE name = 'echo'`); err != nil {
		t.Fatal(err)
	}
	if tl = first(); tl["descriptionDrift"] != true || tl["upstreamDescription"] != "New upstream text" {
		t.Fatalf("drift not detected: %v", tl)
	}
	// Re-saving the same text acknowledges it.
	a.expect(200, "PUT", "/api/v1/virtual-servers/vs", sel("Custom text"))
	if tl = first(); tl["descriptionDrift"] != false {
		t.Fatalf("drift not acknowledged: %v", tl)
	}
	// An empty description removes the override.
	a.expect(200, "PUT", "/api/v1/virtual-servers/vs", sel(""))
	if tl = first(); tl["description"] != nil && tl["description"] != "" || tl["descriptionDrift"] != false {
		t.Fatalf("override not cleared: %v", tl)
	}
	// Too long.
	a.expect(422, "PUT", "/api/v1/virtual-servers/vs", sel(strings.Repeat("x", 2001)))
}
