package config

import (
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/restapi"
	"testing"
)

func TestRESTFingerprintTracksCredentialsWithoutCompiledSchemaInternals(t *testing.T) {
	cfg := restapi.Config{Auth: restapi.Auth{Type: "basic", Username: "u", Password: "one"}, TimeoutSeconds: 30, Operations: []restapi.Operation{{Name: "get", Enabled: true, Method: "GET", Path: "/", ResponseSchema: json.RawMessage(`{"type":"object","patternProperties":{"^x":{"type":"string"}}}`)}}}
	compiled, err := restapi.Compile("https://example.com", cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	one := Upstream{Kind: "rest", URL: "https://example.com", REST: compiled}
	if _, err := json.Marshal(one); err != nil {
		t.Fatal("compiled schema leaked into connection fingerprint", err)
	}
	cfg.Auth.Password = "two"
	compiled, err = restapi.Compile("https://example.com", cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	two := one
	two.REST = compiled
	if one.Fingerprint() == two.Fingerprint() {
		t.Fatal("credential change did not invalidate fingerprint")
	}
	if one.WithPrecomputedFingerprint().Fingerprint() != one.Fingerprint() {
		t.Fatal("unstable fingerprint")
	}
}
