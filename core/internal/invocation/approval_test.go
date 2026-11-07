package invocation

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestApprovalProtocolFailsClosed(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8,::1/128")
	for _, tc := range []struct {
		name, body string
		status     int
		allowed    bool
	}{
		{"allowed", `{"status":"allowed","approvalId":"a","expiresAt":"2099-01-01T00:00:00Z"}`, 200, true},
		{"old-contract", `{"allow":true}`, 200, false},
		{"unknown-fields", `{"status":"allowed","approvalId":"a","expiresAt":"2099-01-01T00:00:00Z","extra":true}`, 200, false},
		{"expired", `{"status":"allowed","approvalId":"a","expiresAt":"2000-01-01T00:00:00Z"}`, 200, false},
		{"wrong-origin", `{"status":"pending","approvalId":"a","expiresAt":"2099-01-01T00:00:00Z","url":"https://evil.test/approve"}`, 200, false},
		{"outage", `{}`, 503, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-service-secret" {
					t.Error("missing service authentication")
				}
				var req Request
				_ = json.NewDecoder(r.Body).Decode(&req)
				if req.Identity.Subject != "alice" {
					t.Error("missing subject")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			cert, err := x509.ParseCertificate(server.TLS.Certificates[0].Certificate[0])
			if err != nil {
				t.Fatal(err)
			}
			svc := Service{URL: server.URL, ApprovalOrigin: server.URL, Token: "test-service-secret", CAPEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}))}
			_, err = Call(context.Background(), svc, Request{Identity: Identity{Subject: "alice"}})
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v err=%v", tc.allowed, err)
			}
		})
	}
}
