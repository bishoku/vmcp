package tenancy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTenantRoutingRejectsHostAndHeaderSpoofing(t *testing.T) {
	a := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer a" || r.Header.Get("X-vMCP-Tenant") != "a" {
			w.WriteHeader(401)
			return
		}
		w.WriteHeader(204)
	}))
	defer a.Close()
	b := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer b" || r.Header.Get("X-vMCP-Tenant") != "b" {
			w.WriteHeader(401)
			return
		}
		w.WriteHeader(204)
	}))
	defer b.Close()
	router, err := New([]Tenant{{ID: "a", Host: "a.example.com", Control: a.URL, Data: a.URL, UI: a.URL}, {ID: "b", Host: "b.example.com", Control: b.URL, Data: b.URL, UI: b.URL}})
	if err != nil {
		t.Fatal(err)
	}
	for _, routes := range router.routes {
		for _, proxy := range routes {
			proxy.Transport = a.Client().Transport
		}
	}
	for _, tc := range []struct {
		host, token string
		want        int
	}{{"a.example.com", "a", 204}, {"b.example.com", "a", 401}, {"b.example.com", "b", 204}, {"evil.example.com", "b", 404}} {
		request := httptest.NewRequest("GET", "https://"+tc.host+"/api/v1/upstreams", nil)
		request.Header.Set("Authorization", "Bearer "+tc.token)
		request.Header.Set("X-vMCP-Tenant", "b")
		request.Header.Set("X-Forwarded-Host", "b.example.com")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != tc.want {
			t.Fatalf("%s: %d", tc.host, response.Code)
		}
	}
}
func TestTenantDirectoryRejectsSharedPlanes(t *testing.T) {
	_, err := New([]Tenant{{ID: "a", Host: "a.example.com", Control: "https://same.example.com", Data: "https://same.example.com", UI: "https://same.example.com"}, {ID: "b", Host: "b.example.com", Control: "https://same.example.com", Data: "https://other.example.com", UI: "https://ui.example.com"}})
	if err == nil {
		t.Fatal("shared tenant planes accepted")
	}
}

func TestTenantRoutingForwardsWellKnownOAuthProtectedResourceToDataPlane(t *testing.T) {
	data := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/oauth-protected-resource/mcp/v1/virtual/srv" && r.Header.Get("X-vMCP-Tenant") == "a" {
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(404)
	}))
	defer data.Close()
	ui := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(418)
	}))
	defer ui.Close()
	router, err := New([]Tenant{{ID: "a", Host: "a.example.com", Control: ui.URL, Data: data.URL, UI: ui.URL}})
	if err != nil {
		t.Fatal(err)
	}
	router.routes["a.example.com"]["data"].Transport = data.Client().Transport
	router.routes["a.example.com"]["ui"].Transport = ui.Client().Transport
	req := httptest.NewRequest("GET", "https://a.example.com/.well-known/oauth-protected-resource/mcp/v1/virtual/srv", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200 from data plane for well-known resource metadata, got %d", rec.Code)
	}
}

