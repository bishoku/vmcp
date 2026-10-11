package virtual

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApprovalPolicyFailClosedAndMetadataOnly(t *testing.T) {
	for _, body := range []string{`{"allow":true}`, `{"allow":false}`, `{}`, `{"allow":true,"extra":1}`, `{"allow":true} {}`, strings.Repeat("x", 4097)} {
		t.Run(body[:min(len(body), 32)], func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, _ := io.ReadAll(r.Body)
				if strings.Contains(string(raw), "private-payload") {
					t.Error("payload escaped to approval hook")
				}
				var d PolicyDecision
				if json.Unmarshal(raw, &d) != nil || len(d.Digest) != 64 || d.Actor != "actor" {
					t.Error("missing attestation")
				}
				w.Write([]byte(body))
			}))
			defer server.Close()
			p := &ApprovalPolicy{endpoint: server.URL, client: server.Client()}
			err := p.Check(context.Background(), PolicyDecision{Actor: "actor"}, []byte("private-payload"))
			if (err == nil) != (body == `{"allow":true}`) {
				t.Fatal("approval failed closed incorrectly")
			}
		})
	}
	p := &ApprovalPolicy{endpoint: "https://127.0.0.1:1", client: &http.Client{}}
	if p.Check(context.Background(), PolicyDecision{}, nil) == nil {
		t.Fatal("outage allowed")
	}
	if _, err := NewApprovalPolicy("http://example.com"); err == nil {
		t.Fatal("insecure hook accepted")
	}
}
