package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	fx "github.com/bishoku/vmcp/core/internal/fixture"
	"github.com/bishoku/vmcp/core/internal/tlsconf"
)

func TestSSEClientAgainstLegacyFixture(t *testing.T) {
	srv := httptest.NewServer(fx.NewLegacySSE(fx.Options{Token: "s3"}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Wrong token is rejected.
	if _, err := NewSSEClient(srv.URL, nil, nil).Initialize(ctx); err == nil {
		t.Fatal("expected auth failure")
	}

	c, err := Open("sse", srv.URL, map[string]string{"Authorization": "Bearer s3"}, tlsconf.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	init, err := c.Initialize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(init.ServerInfo.Name, "legacy") {
		t.Fatalf("server=%q", init.ServerInfo.Name)
	}
	tools, err := c.ListTools(ctx)
	if err != nil || len(tools) == 0 {
		t.Fatalf("tools=%v err=%v", tools, err)
	}
	raw, err := c.CallTool(ctx, "add", json.RawMessage(`{"a":2,"b":40}`))
	if err != nil || !strings.Contains(string(raw), "42") {
		t.Fatalf("raw=%s err=%v", raw, err)
	}

	// Stream closure surfaces as an expired session so callers can reconnect.
	_ = c.Close(ctx)
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, err = c.CallTool(ctx, "add", json.RawMessage(`{"a":1,"b":1}`))
		if err == ErrSessionExpired {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("err=%v, want ErrSessionExpired", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Rewrite only the protocol returned by the real legacy SSE fixture.
type versionWriter struct {
	http.ResponseWriter
	http.Flusher
	version string
}

func (w versionWriter) Write(b []byte) (int, error) {
	_, err := w.ResponseWriter.Write(bytes.ReplaceAll(b, []byte("2025-06-18"), []byte(w.version)))
	if err != nil {
		return 0, err
	}
	return len(b), nil
}
func TestLegacySSERejectsUnsupportedVersion(t *testing.T) {
	for _, version := range []string{"2024-11-05", "2025-03-26", "2099-01-01"} {
		t.Run(version, func(t *testing.T) {
			fixture := fx.NewLegacySSE(fx.Options{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fixture.ServeHTTP(versionWriter{w, w.(http.Flusher), version}, r)
			}))
			defer srv.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			c := NewSSEClient(srv.URL, nil, nil)
			if _, err := c.Initialize(ctx); err == nil || !strings.Contains(err.Error(), "unsupported upstream protocol version") {
				t.Fatalf("unsupported version: %v", err)
			}
		})
	}
}

func TestSSEEndpointCannotExfiltrateCredentials(t *testing.T) {
	var hits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer target.Close()
	for _, endpoint := range []string{target.URL + "/messages", "http://169.254.169.254/latest/meta-data/", "https://user:password@example.com/messages"} {
		t.Run(endpoint, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "event: endpoint\ndata: "+endpoint+"\n\n")
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			defer srv.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			c := NewSSEClient(srv.URL, map[string]string{"Authorization": "Bearer secret", "X-API-Key": "private"}, nil)
			if _, err := c.Initialize(ctx); err == nil || !strings.Contains(err.Error(), "same origin") {
				t.Fatalf("unsafe endpoint: %v", err)
			}
		})
	}
	if hits.Load() != 0 {
		t.Fatal("credentials sent to cross-origin endpoint")
	}
}

func TestMCPClientCannotDialUnapprovedLoopback(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer srv.Close()
	for _, transport := range []string{"streamable_http", "sse"} {
		c, err := Open(transport, srv.URL, nil, tlsconf.Settings{})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, err = c.Initialize(ctx)
		cancel()
		if err == nil {
			t.Fatalf("unapproved loopback reached through %s", transport)
		}
	}
	if hits.Load() != 0 {
		t.Fatal("blocked upstream received a request")
	}
}

func TestOpenRejectsNonHTTPTransports(t *testing.T) {
	for _, transport := range []string{"process", "file", "unknown"} {
		if conn, err := Open(transport, "https://example.com/mcp", nil, tlsconf.Settings{}); err == nil || conn != nil {
			t.Fatalf("unsupported transport %q accepted", transport)
		}
	}
}
