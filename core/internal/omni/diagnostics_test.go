package omni

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestEmbeddingDiagnosticsAndRedaction(t *testing.T) {
	for _, tc := range []struct {
		name, body, reason string
		status             int
	}{
		{"unauthorized", `{"error":{"code":"invalid_api_key","type":"authentication_error","message":"TOPSECRET PRIVATEQUERY"}}`, "http_status", 401},
		{"rate_limit", `{"error":{"code":"rate_limit_exceeded"}}`, "http_status", 429},
		{"html", "<html>TOPSECRET PRIVATEQUERY</html>", "invalid_json", 200},
		{"empty", "", "empty_response", 200},
		{"count", `{"data":[]}`, "embedding_count_mismatch", 200},
		{"dimension", `{"data":[{"index":0,"embedding":[]}]}`, "invalid_embedding_dimensions", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			e, _ := NewHTTPEmbedder(srv.URL+"/TOPSECRET?key=TOPSECRET", "model", "TOPSECRET", false)
			defer e.Client.CloseIdleConnections()
			_, err := e.Embed(context.Background(), []string{"PRIVATEQUERY"})
			var diagnostic *embeddingError
			if !errors.As(err, &diagnostic) || diagnostic.reason != tc.reason {
				t.Fatalf("unexpected diagnostic: %v", err)
			}
			var buf bytes.Buffer
			slog.New(slog.NewJSONHandler(&buf, nil)).Warn("failure", "error", diagnostic)
			log := buf.String()
			for _, secret := range []string{"TOPSECRET", "PRIVATEQUERY", "message"} {
				if strings.Contains(log, secret) {
					t.Fatalf("sensitive content in log: %s", log)
				}
			}
			if tc.status != 200 && !strings.Contains(log, `"http_status":`) {
				t.Fatal(log)
			}
			if tc.name == "unauthorized" && !strings.Contains(log, `"provider_code":"invalid_api_key"`) {
				t.Fatal(log)
			}
		})
	}
}

func TestEmbeddingTransportDiagnostics(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	e, _ := NewHTTPEmbedder(srv.URL, "model", "", false)
	defer e.Client.CloseIdleConnections()
	_, err := e.Embed(context.Background(), []string{"query"})
	var diagnostic *embeddingError
	if !errors.As(err, &diagnostic) || diagnostic.reason != "tls_unknown_authority" {
		t.Fatalf("TLS cause lost: %v", err)
	}
	for _, tc := range []struct {
		err  error
		want string
	}{
		{context.DeadlineExceeded, "timeout"}, {context.Canceled, "request_canceled"},
		{&net.DNSError{Err: "lookup failed", Name: "private"}, "dns_lookup_failed"},
		{&net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, "connection_refused"},
	} {
		if got := transportReason(tc.err); got != tc.want {
			t.Fatalf("%s != %s", got, tc.want)
		}
	}
}

func TestIndexLogsFallbackThrottleAndRecovery(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(old)
	fail := true
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[1,0]}]}`))
	}))
	defer provider.Close()
	doc := Document{Ref: Ref("chat", "orders"), Server: "chat", Name: "orders"}
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Catalog{Documents: []Document{doc}})
	}))
	defer catalog.Close()
	b, _ := NewBackend(catalog.URL, strings.Repeat("x", 32))
	e, _ := NewHTTPEmbedder(provider.URL, "model", "", false)
	defer e.Client.CloseIdleConnections()
	queryIndex := NewIndex(e)
	fail = false
	queryIndex.Refresh(context.Background(), b)
	fail = true
	index := NewIndex(e)
	index.Refresh(context.Background(), b)
	if index.Status().Mode != "hybrid-degraded" {
		t.Fatal(index.Status())
	}
	access := Access{Refs: map[string]string{doc.Ref: doc.Hash()}}
	for n := 0; n < 3; n++ {
		_, mode, err := queryIndex.Search(context.Background(), access, "orders", "", "", 1)
		if err != nil || mode != "keyword-fallback" {
			t.Fatalf("%s %v", mode, err)
		}
	}
	if strings.Count(buf.String(), `"msg":"omni dependency failed"`) != 2 {
		t.Fatal(buf.String())
	}
	queryIndex.searchLog.last = time.Now().Add(-31 * time.Second)
	_, _, _ = queryIndex.Search(context.Background(), access, "orders", "", "", 1)
	if !strings.Contains(buf.String(), `"suppressed_failures":2`) {
		t.Fatal(buf.String())
	}
	fail = false
	index.Refresh(context.Background(), b)
	_, mode, err := queryIndex.Search(context.Background(), access, "orders", "", "", 1)
	if err != nil || mode != "hybrid" {
		t.Fatalf("%s %v", mode, err)
	}
	log := buf.String()
	if strings.Count(log, `"msg":"omni dependency recovered"`) != 2 || !strings.Contains(log, `"operation":"index_refresh"`) || !strings.Contains(log, `"operation":"search_embedding"`) {
		t.Fatal(log)
	}
}
