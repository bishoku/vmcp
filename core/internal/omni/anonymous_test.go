package omni

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestAnonymousSearchQuotaUsesVerifiedIP(t *testing.T) {
	s := &Server{SearchLimit: 1, Index: NewIndex(nil), Backend: &Backend{Secret: "secret"}, TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}}
	_ = s.Handler()
	allow := func(remote, forwarded, token string) bool {
		r := httptest.NewRequest("POST", "/mcp/v1/omni", nil)
		r.RemoteAddr = remote
		r.Header.Set("X-Forwarded-For", forwarded)
		return s.allowSearch(httptest.NewRecorder(), r, token)
	}
	if !allow("192.0.2.1:80", "", "") || !allow("192.0.2.2:80", "", "") {
		t.Fatal("different IPs share anonymous quota")
	}
	if allow("192.0.2.1:80", "198.51.100.1", "") {
		t.Fatal("untrusted proxy spoof bypassed quota")
	}
	if !allow("10.0.0.1:80", "198.51.100.1", "") || allow("10.0.0.2:80", "198.51.100.1", "") {
		t.Fatal("trusted proxy addresses used instead of client IP")
	}
	if !allow("192.0.2.1:80", "", "Bearer one") || allow("192.0.2.2:80", "", "Bearer one") {
		t.Fatal("credential quota changed with IP")
	}
}
