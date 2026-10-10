package virtual

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIngressBoundaryMatrix(t *testing.T) {
	t.Setenv("VMCP_ALLOWED_ORIGINS", "https://console.example,https://localhost:3000")
	for _, method := range []string{"POST", "GET", "DELETE"} {
		for _, tc := range []struct {
			name, origin, version string
			want                  int
		}{
			{"native", "", "", 204}, {"approved", "https://console.example", "2025-06-18", 204},
			{"default port", "https://console.example:443", "", 204},
			{"host rebinding", "https://attacker.example", "", 403}, {"null", "null", "", 403},
			{"userinfo", "https://user@console.example", "", 403}, {"path", "https://console.example/path", "", 403},
			{"multiple origins", "https://console.example, https://attacker.example", "", 403},
			{"unsupported", "", "2025-03-26", 400}, {"unknown", "", "2099-01-01", 400},
		} {
			t.Run(method+"/"+tc.name, func(t *testing.T) {
				req := httptest.NewRequest(method, "http://gateway/mcp", strings.NewReader(`{}`))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Accept", "application/json, text/event-stream")
				if tc.origin != "" {
					req.Header.Set("Origin", tc.origin)
				}
				if tc.version != "" {
					req.Header.Set("MCP-Protocol-Version", tc.version)
				}
				response := httptest.NewRecorder()
				ingress(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })(response, req)
				if response.Code != tc.want {
					t.Fatalf("got %d %s", response.Code, response.Body.String())
				}
			})
		}
	}
	for _, header := range []string{"Origin", "MCP-Protocol-Version"} {
		req := httptest.NewRequest("DELETE", "http://gateway/mcp", nil)
		req.Header.Add(header, "x")
		req.Header.Add(header, "y")
		w := httptest.NewRecorder()
		ingress(func(http.ResponseWriter, *http.Request) { t.Fatal("duplicate header passed") })(w, req)
		if w.Code < 400 {
			t.Fatal("duplicate header accepted")
		}
	}
}
func TestIngressMediaAndBodyBudgets(t *testing.T) {
	t.Setenv("VMCP_ALLOWED_ORIGINS", "")
	for _, tc := range []struct {
		media, accept string
		want          int
	}{
		{"application/json", "application/json, text/event-stream", 204},
		{"application/json; charset=utf-8", "application/json;q=0.5, text/event-stream", 204},
		{"text/plain", "application/json, text/event-stream", 415},
		{"application/json; charset=latin1", "application/json, text/event-stream", 415},
		{"application/json", "application/json", 406}, {"application/json", "*/*", 406},
		{"application/json", "application/json, text/event-stream;q=0", 406},
		{"application/json", "application/json, text/event-stream;q=NaN", 406},
	} {
		t.Run(tc.media+tc.accept, func(t *testing.T) {
			req := httptest.NewRequest("POST", "http://gateway/mcp", nil)
			req.Header.Set("Content-Type", tc.media)
			req.Header.Set("Accept", tc.accept)
			w := httptest.NewRecorder()
			ingress(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })(w, req)
			if w.Code != tc.want {
				t.Fatalf("got %d want %d", w.Code, tc.want)
			}
		})
	}
	r := newRig(t)
	req := httptest.NewRequest("POST", "/mcp/v1/virtual/support", bytes.NewReader(bytes.Repeat([]byte(" "), maxRequestBytes+1)))
	req.SetPathValue("slug", "support")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	w := httptest.NewRecorder()
	ingress(r.handler.post)(w, req)
	if w.Code != 413 {
		t.Fatalf("oversized body: %d", w.Code)
	}
}
func TestOriginDefaultAndConfiguration(t *testing.T) {
	for _, config := range []string{"", "not-an-origin", "https://console.example/"} {
		t.Setenv("VMCP_ALLOWED_ORIGINS", config)
		req := httptest.NewRequest("DELETE", "http://gateway", nil)
		req.Header.Set("Origin", "https://console.example")
		w := httptest.NewRecorder()
		ingress(func(http.ResponseWriter, *http.Request) { t.Fatal("unapproved origin passed") })(w, req)
		if config == "" && w.Code != 403 {
			t.Fatal("empty list permits browser origin")
		}
		if config != "" && w.Code != 503 {
			t.Fatal("invalid config did not fail closed")
		}
	}
}
func TestGETAuthenticatesAndChecksSessionOwnership(t *testing.T) {
	t.Setenv("VMCP_ALLOWED_ORIGINS", "")
	r := newRig(t)
	sid := r.initialize(t)
	for _, tc := range []struct {
		sid  string
		want int
	}{{sid, 405}, {"unknown", 404}} {
		req, _ := http.NewRequest("GET", r.gw.URL+"/mcp/v1/virtual/support", nil)
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Mcp-Session-Id", tc.sid)
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != tc.want {
			t.Fatalf("GET %s: %d", tc.sid, response.StatusCode)
		}
	}
	// A protected virtual server must authenticate even though streaming is unsupported.
	locked := authSetup(t)
	req, _ := http.NewRequest("GET", locked.URL+"/mcp/v1/virtual/locked", nil)
	req.Header.Set("Accept", "text/event-stream")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatalf("unauthenticated GET: %d", response.StatusCode)
	}
}

func TestSessionOwnershipAcrossMethods(t *testing.T) {
	srv := authSetup(t)
	response, _ := post(t, srv.URL+"/mcp/v1/virtual/locked", initBody, map[string]string{"Authorization": "Bearer vmcp_good"})
	sid := response.Header.Get("Mcp-Session-Id")
	if sid == "" {
		t.Fatal("no session")
	}
	for _, method := range []string{"POST", "GET", "DELETE"} {
		req, _ := http.NewRequest(method, srv.URL+"/mcp/v1/virtual/locked", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"ping"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer vmcp_other")
		req.Header.Set("Mcp-Session-Id", sid)
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 404 {
			t.Fatalf("%s allowed another principal's session: %d", method, response.StatusCode)
		}
	}
}

func TestAllRegisteredMCPRoutesUseIngress(t *testing.T) {
	t.Setenv("VMCP_ALLOWED_ORIGINS", "")
	r := newRig(t)
	for _, method := range []string{"POST", "GET", "DELETE"} {
		for _, badOrigin := range []bool{true, false} {
			req, _ := http.NewRequest(method, r.gw.URL+"/mcp/v1/virtual/support", strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			want := 400
			if badOrigin {
				req.Header.Set("Origin", "https://attacker.example")
				want = 403
			} else {
				req.Header.Set("MCP-Protocol-Version", "2025-03-26")
			}
			response, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != want {
				t.Fatalf("%s did not use ingress validation: %d", method, response.StatusCode)
			}
		}
	}
}
