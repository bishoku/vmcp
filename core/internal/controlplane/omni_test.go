package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/bishoku/vmcp/core/internal/apierror"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOmniPreviewUsesExplicitRuntimeIdentity(t *testing.T) {
	secret := strings.Repeat("s", 32)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/preview" || r.Header.Get("Authorization") != "Bearer runtime-key" || r.Header.Get("X-VMCP-Omni-Secret") != secret {
			t.Errorf("incorrect preview forwarding")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["token"]; ok {
			t.Error("credential in audit-visible body")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"tools": []any{}})
	}))
	defer backend.Close()
	t.Setenv("VMCP_OMNI_URL", backend.URL)
	t.Setenv("VMCP_OMNI_SECRET", secret)
	s := &Server{}
	req := httptest.NewRequest("POST", "/api/v1/omni/preview", strings.NewReader(`{"query":"orders"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer management-key")
	if err := s.omni(httptest.NewRecorder(), req); err == nil {
		t.Fatal("management identity implicitly reused")
	}
	req = httptest.NewRequest("POST", "/api/v1/omni/preview", strings.NewReader(`{"query":"orders"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-VMCP-Runtime-Authorization", "Bearer runtime-key")
	if err := s.omni(httptest.NewRecorder(), req); err != nil {
		t.Fatal(err)
	}
}
func TestOmniAdminControlsAndStatusIsolation(t *testing.T) {
	t.Setenv("VMCP_OMNI_URL", "http://not-contacted.invalid")
	s := &Server{}
	r := httptest.NewRequest("GET", "/api/v1/omni", nil)
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, &principal{Role: "viewer"}))
	w := httptest.NewRecorder()
	if err := s.omni(w, r); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(w.Body.String(), "documents") {
		t.Fatal("global index counts exposed")
	}
}

func TestOmniRuntimeDenialIsDistinctFromConsoleAuthorization(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
			defer backend.Close()
			t.Setenv("VMCP_OMNI_URL", backend.URL)
			t.Setenv("VMCP_OMNI_SECRET", strings.Repeat("s", 32))
			req := httptest.NewRequest("POST", "/api/v1/omni/preview", strings.NewReader(`{"query":"orders"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-VMCP-Runtime-Authorization", "Bearer denied-runtime-key")
			err := (&Server{}).omni(httptest.NewRecorder(), req)
			var problem *apierror.Error
			if !errors.As(err, &problem) || problem.Status != http.StatusForbidden || problem.Code != "runtime_access_denied" {
				t.Fatal("runtime denial lost its boundary", err)
			}
		})
	}
}
