package restapi

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/bishoku/vmcp/core/internal/tlsconf"
)

// Lose a response after execution on a reused connection: never repeat the write.
func TestLostResponseNeverReplays(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	for _, http2 := range []bool{false, true} {
		for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
			for _, header := range []string{"", "Idempotency-Key", "X-Idempotency-Key"} {
				t.Run(fmt.Sprintf("http2=%v/%s/%s", http2, method, header), func(t *testing.T) {
					var calls atomic.Int32
					var warmAddress atomic.Value
					var srv *httptest.Server
					srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						_, _ = io.Copy(io.Discard, r.Body)
						if r.URL.Path == "/fail" {
							if calls.Add(1) == 1 && warmAddress.Load() != r.RemoteAddr {
								t.Error("connection pooling was not preserved")
							}
							if http2 {
								if r.ProtoMajor != 2 {
									t.Error("HTTP/2 was not negotiated")
								}
								srv.CloseClientConnections()
								return
							}
							conn, _, err := w.(http.Hijacker).Hijack()
							if err != nil {
								t.Error(err)
								return
							}
							_ = conn.Close()
							return
						}
						warmAddress.Store(r.RemoteAddr)
						_, _ = w.Write([]byte(`{}`))
					}))
					srv.EnableHTTP2 = http2
					if http2 {
						srv.StartTLS()
					} else {
						srv.Start()
					}
					defer srv.Close()
					tl := tlsconf.Settings{}
					if http2 {
						tl.CAPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}))
					}
					cfg := Config{Auth: Auth{Type: "none"}, AllowInsecureHTTP: true, TimeoutSeconds: 3,
						Operations: []Operation{{Name: "warm", Enabled: true, Method: "GET", Path: "/warm"}, {Name: "fail", Enabled: true, Method: method, Path: "/fail"}}}
					args := json.RawMessage(`{}`)
					if method == "POST" || method == "PUT" || method == "PATCH" {
						cfg.Operations[1].BodySchema = json.RawMessage(`{"type":"object"}`)
						args = json.RawMessage(`{"body":{"value":1}}`)
					}
					headers := map[string]string{}
					if header != "" {
						headers[header] = "operator-configured"
					}
					c := compile(t, srv.URL, cfg)
					if _, err := c.Execute(context.Background(), srv.URL, "warm", json.RawMessage(`{}`), nil, tl, Identity{}, nil); err != nil {
						t.Fatal(err)
					}
					if _, err := c.Execute(context.Background(), srv.URL, "fail", args, headers, tl, Identity{}, nil); err == nil {
						t.Fatal("lost response must fail")
					}
					if calls.Load() != 1 {
						t.Fatalf("operation executed %d times", calls.Load())
					}
				})
			}
		}
	}
}
