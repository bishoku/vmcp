package controlplane

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMandatoryAuthorizationFailsClosedWithoutDatabase(t *testing.T) {
	s := &Server{AdminToken: "test-bootstrap"}
	called := false
	handler := s.wrap(func(w http.ResponseWriter, r *http.Request) error { called = true; w.WriteHeader(200); return nil })
	r := httptest.NewRequest("GET", "/api/v1/upstreams?mode=legacy", nil)
	r.Pattern = "GET /api/v1/upstreams"
	r.Header.Set("Authorization", "Bearer test-bootstrap")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, r)
	if called || response.Code != 503 {
		t.Fatal("authorization unavailable bypass", called, response.Code, response.Body.String())
	}
}
