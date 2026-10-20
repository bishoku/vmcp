package restapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/bishoku/vmcp/core/internal/tlsconf"
)

func baseConfig() Config {
	return Config{Auth: Auth{Type: "none"}, TimeoutSeconds: 3, AllowInsecureHTTP: true, Operations: []Operation{{Name: "lookup", Enabled: true, Method: "GET", Path: "/items/{id}", ReadOnly: true, Parameters: []Parameter{{Name: "id", In: "path", Schema: json.RawMessage(`{"type":"integer"}`)}, {Name: "q", In: "query", Schema: json.RawMessage(`{"type":"string"}`)}}, ResponseSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"]}`)}}}
}
func compile(t *testing.T, base string, c Config) *Compiled {
	t.Helper()
	out, err := Compile(base, c, nil)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func TestMappingPrecisionAndProjection(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	c := baseConfig()
	c.ForwardIdentity = true
	c.Auth = Auth{Type: "basic", Username: "user", Password: "secret"}
	c.Operations[0].ResponseFields = []string{"id"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/items/9007199254740993" || r.URL.Query().Get("q") != "a&b" {
			t.Errorf("wrong mapping: %s", r.URL)
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "user" || pass != "secret" {
			t.Error("missing Basic Auth")
		}
		if r.Header.Get("X-VMCP-Agent-Id") != "key-id" || r.Header.Get("X-VMCP-Virtual-Server") != "agent" || r.Header.Get("X-Api-Key") != "manual" {
			t.Error("wrong headers")
		}
		_, _ = w.Write([]byte(`{"id":9007199254740993,"hidden":"not-for-agent"}`))
	}))
	defer srv.Close()
	compiled := compile(t, srv.URL+"/api", c)
	out, err := compiled.Execute(context.Background(), srv.URL+"/api", "lookup", json.RawMessage(`{"id":9007199254740993,"q":"a&b"}`), map[string]string{"X-Api-Key": "manual"}, tlsconf.Settings{}, Identity{Type: "agent-key", ID: "key-id", VirtualServer: "agent"}, nil)
	if err != nil || !strings.Contains(string(out), "9007199254740993") || strings.Contains(string(out), "not-for-agent") || strings.Contains(string(out), "isError") {
		t.Fatalf("%s %v", out, err)
	}
}
func TestValidationAndSecretRetention(t *testing.T) {
	previous := baseConfig()
	previous.Auth = Auth{Type: "basic", Username: "a", Password: "private"}
	previous.Operations[0].Headers = map[string]string{"X-Key": "private"}
	public := previous.Public()
	raw, _ := json.Marshal(public)
	if strings.Contains(string(raw), "private") || !public.Auth.HasPassword {
		t.Fatal(string(raw))
	}
	resolved := Resolve(public, &previous)
	if resolved.Auth.Password != "private" || resolved.Operations[0].Headers["X-Key"] != "private" {
		t.Fatal("omitted secrets lost")
	}
	public.Operations[0].Headers = map[string]string{}
	if len(Resolve(public, &previous).Operations[0].Headers) != 0 {
		t.Fatal("explicit clear lost")
	}
	for _, header := range []string{"X-vMcp-Agent-Id", "Host", "Content-Length", "Authorization", "Connection"} {
		if ValidateHeaders(map[string]string{header: "x"}, true) == nil {
			t.Fatalf("allowed %s", header)
		}
	}
	for _, mutate := range []func(*Config){func(c *Config) { c.AllowInsecureHTTP = false }, func(c *Config) { c.Operations[0].Path = "//evil" }, func(c *Config) { c.Operations[0].Path = "/../secret" }, func(c *Config) {
		c.Operations[0].ResponseSchema = json.RawMessage(`{"$ref":"https://example.com/schema"}`)
	}, func(c *Config) { c.Operations[0].Method = "POST" }, func(c *Config) { c.RequireIdentity = true }} {
		c := baseConfig()
		mutate(&c)
		if _, err := Compile("http://example.com", c, nil); err == nil {
			t.Fatal("unsafe config accepted")
		}
	}
	compiled := compile(t, "https://example.com", baseConfig())
	for _, args := range []string{`{}`, `{"id":"string"}`, `{"id":1,"X-VMCP-Agent-Id":"spoof"}`, `{"id":1} {}`} {
		if _, err := compiled.Build("https://example.com", "lookup", json.RawMessage(args), nil, Identity{}); err == nil {
			t.Fatal("invalid args accepted")
		}
	}
}
func TestOAuthNoReplayAndTokenAuthMethods(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	for _, method := range []string{"client_secret_basic", "client_secret_post"} {
		t.Run(method, func(t *testing.T) {
			var tokens, calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/token" {
					tokens.Add(1)
					_ = r.ParseForm()
					if r.Form.Get("grant_type") != "client_credentials" {
						t.Error("wrong grant")
					}
					if method == "client_secret_basic" {
						u, p, ok := r.BasicAuth()
						if !ok || u != "client" || p != "secret" {
							t.Error("wrong token header")
						}
					} else if r.Form.Get("client_id") != "client" || r.Form.Get("client_secret") != "secret" || r.Header.Get("Authorization") != "" {
						t.Error("wrong token body")
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"access_token":"cached","token_type":"Bearer","expires_in":3600}`))
					return
				}
				calls.Add(1)
				if r.Header.Get("Authorization") != "Bearer cached" {
					t.Error("missing token")
				}
				w.WriteHeader(401)
				_, _ = w.Write([]byte("private backend error"))
			}))
			defer srv.Close()
			c := baseConfig()
			c.Auth = Auth{Type: "oauth2", TokenURL: srv.URL + "/token", ClientID: "client", ClientSecret: "secret", TokenAuthMethod: method}
			compiled := compile(t, srv.URL, c)
			out, err := compiled.Execute(context.Background(), srv.URL, "lookup", json.RawMessage(`{"id":1}`), nil, tlsconf.Settings{}, Identity{}, nil)
			if err != nil || calls.Load() != 1 || tokens.Load() != 1 || !strings.Contains(string(out), "401") || strings.Contains(string(out), "private") {
				t.Fatalf("out=%s err=%v requests=%d tokens=%d", out, err, calls.Load(), tokens.Load())
			}
			denied := errors.New("revoked")
			_, err = compiled.Execute(context.Background(), srv.URL, "lookup", json.RawMessage(`{"id":1}`), nil, tlsconf.Settings{}, Identity{}, func() error { return denied })
			if !errors.Is(err, denied) || calls.Load() != 1 {
				t.Fatal("revoked request dispatched")
			}
		})
	}
}
func TestRedirectEgressAndResponseFailures(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Query().Get("q") {
		case "redirect":
			http.Redirect(w, r, "/target", 302)
		case "large":
			_, _ = w.Write([]byte(strings.Repeat("x", MaxResponseBytes+1)))
		case "invalid":
			_, _ = w.Write([]byte("private invalid body"))
		case "schema":
			_, _ = w.Write([]byte(`{"id":"wrong"}`))
		default:
			_, _ = w.Write([]byte(`{"id":1}`))
		}
	}))
	defer srv.Close()
	c := compile(t, srv.URL, baseConfig())
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "")
	t.Setenv("VMCP_EGRESS_ALLOW_HOSTS", "")
	if _, err := c.Execute(context.Background(), srv.URL, "lookup", json.RawMessage(`{"id":1}`), nil, tlsconf.Settings{}, Identity{}, nil); err == nil || calls.Load() != 0 {
		t.Fatal("private egress allowed")
	}
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	for _, q := range []string{"redirect", "large", "invalid", "schema"} {
		before := calls.Load()
		out, err := c.Execute(context.Background(), srv.URL, "lookup", json.RawMessage(`{"id":1,"q":"`+q+`"}`), nil, tlsconf.Settings{}, Identity{}, nil)
		if err != nil || !strings.Contains(string(out), `"isError":true`) || strings.Contains(string(out), "private") || calls.Load() != before+1 {
			t.Fatalf("%s: %s %v", q, out, err)
		}
	}
}
func TestIdentityAndPreview(t *testing.T) {
	c := baseConfig()
	c.ForwardIdentity = true
	c.RequireIdentity = true
	c.Auth = Auth{Type: "bearer", Token: "secret"}
	compiled := compile(t, "https://example.com", c)
	if _, err := compiled.Build("https://example.com", "lookup", json.RawMessage(`{"id":1}`), nil, Identity{}); err == nil {
		t.Fatal("anonymous accepted")
	}
	preview, err := compiled.Preview("https://example.com", "lookup", json.RawMessage(`{"id":1}`), map[string]string{"X-Key": "private"})
	raw, _ := json.Marshal(preview)
	if err != nil || strings.Contains(string(raw), "private") || strings.Contains(string(raw), "Bearer secret") {
		t.Fatalf("%s %v", raw, err)
	}
	a := Identity{Type: "oauth", Tenant: "tenant1", Issuer: "issuer", Subject: "subject"}
	b := a
	b.Tenant = "tenant2"
	if a.AgentID() == b.AgentID() || a.AgentID() == "" || strings.Contains(a.AgentID(), "subject") {
		t.Fatal("identity not scoped")
	}
}
func TestBodySchemaAndNoContent(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.Number
		d := json.NewDecoder(r.Body)
		d.UseNumber()
		if d.Decode(&body) != nil || body["id"] != "9007199254740993" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("body changed")
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()
	c := baseConfig()
	c.Operations = []Operation{{Name: "write", Enabled: true, Method: "POST", Path: "/write", BodySchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"]}`), ResponseSchema: json.RawMessage(`{"type":"null"}`)}}
	compiled := compile(t, srv.URL, c)
	out, err := compiled.Execute(context.Background(), srv.URL, "write", json.RawMessage(`{"body":{"id":9007199254740993}}`), nil, tlsconf.Settings{}, Identity{}, nil)
	if err != nil || strings.Contains(string(out), "isError") || !strings.Contains(string(out), `"result":null`) {
		t.Fatalf("%s %v", out, err)
	}
}
