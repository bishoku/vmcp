package controlplane

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPermitRoles(t *testing.T) {
	req := func(method, path string) *http.Request { return httptest.NewRequest(method, path, nil) }
	cases := []struct {
		role, method, path string
		ok                 bool
	}{
		{"viewer", "GET", "/api/v1/upstreams", true},
		{"viewer", "POST", "/api/v1/upstreams", false},
		{"editor", "POST", "/api/v1/upstreams", true},
		{"editor", "GET", "/api/v1/admin-tokens", true},
		{"editor", "POST", "/api/v1/admin-tokens", false},
		{"editor", "DELETE", "/api/v1/admin-tokens/x", false},
		{"admin", "POST", "/api/v1/admin-tokens", true},
	}
	for _, c := range cases {
		if err := permit(c.role, req(c.method, c.path)); (err == nil) != c.ok {
			t.Errorf("%s %s %s: err=%v want ok=%v", c.role, c.method, c.path, err, c.ok)
		}
	}
}
