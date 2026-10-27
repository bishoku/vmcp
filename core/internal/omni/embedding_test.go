package omni

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmbeddingSelfSignedTLSRequiresExplicitOptIn(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing provider credential")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[1,0]}]}`))
	}))
	defer server.Close()
	for _, insecure := range []bool{false, true, false} {
		embedder, err := NewHTTPEmbedder(server.URL, "test-model", "test-key", insecure)
		if err != nil {
			t.Fatal(err)
		}
		defer embedder.Client.CloseIdleConnections()
		vectors, err := embedder.Embed(context.Background(), []string{"query"})
		if !insecure {
			if err == nil {
				t.Fatal("self-signed server accepted without opt-in")
			}
		} else if err != nil || len(vectors) != 1 || vectors[0][0] != 1 {
			t.Fatalf("explicit insecure request failed: vectors=%v error=%v", vectors, err)
		}
	}
	// The opt-in is confined to the embedding client's transport.
	if c := http.DefaultTransport.(*http.Transport).TLSClientConfig; c != nil && c.InsecureSkipVerify {
		t.Fatal("embedding opt-in changed the global TLS configuration")
	}
}
