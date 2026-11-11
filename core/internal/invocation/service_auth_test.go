package invocation

import (
	"context"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func serviceFixture(t *testing.T, handler http.HandlerFunc) Service {
	t.Helper()
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8,::1/128")
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	return Service{URL: server.URL + "/decision", ApprovalOrigin: server.URL, CAPEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))}
}
func decision(w http.ResponseWriter) {
	fmt.Fprint(w, `{"status":"allowed","approvalId":"test","expiresAt":"2099-01-01T00:00:00Z"}`)
}
func TestServiceAuthenticationModes(t *testing.T) {
	for _, mode := range []string{"bearer", "none", "headers"} {
		t.Run(mode, func(t *testing.T) {
			svc := serviceFixture(t, func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "bearer":
					if r.Header.Get("Authorization") != "Bearer secret" {
						t.Error("missing bearer")
					}
				case "none":
					if r.Header.Get("Authorization") != "" {
						t.Error("unexpected credentials")
					}
				case "headers":
					if r.Header.Get("X-API-Key") != "key" || r.Header.Get("Authorization") != "Custom value" {
						t.Error("missing headers")
					}
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Error("content type changed")
				}
				decision(w)
			})
			svc.AuthType = mode
			if mode == "bearer" {
				svc.Token = "secret"
			}
			if mode == "headers" {
				svc.Headers = map[string]string{"X-API-Key": "key", "Authorization": "Custom value"}
			}
			if _, err := Call(context.Background(), svc, Request{Phase: "consume"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestServiceOAuthCacheRotationAndNoReplay(t *testing.T) {
	for _, style := range []string{"client_secret_basic", "client_secret_post"} {
		t.Run(style, func(t *testing.T) {
			var fetches, calls atomic.Int32
			var reject atomic.Bool
			svc := serviceFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/token" {
					fetches.Add(1)
					_ = r.ParseForm()
					id, secret, ok := r.BasicAuth()
					if style == "client_secret_post" {
						id, secret, ok = r.Form.Get("client_id"), r.Form.Get("client_secret"), true
					}
					if !ok || id != "client" || (secret != "secret" && secret != "rotated") || r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("scope") != "approval" || r.Form.Get("audience") != "decision" {
						t.Error("invalid token request")
					}
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"access_token":"cached","token_type":"Bearer","expires_in":3600}`)
					return
				}
				calls.Add(1)
				if r.Header.Get("Authorization") != "Bearer cached" {
					t.Error("missing access token")
				}
				if reject.Load() {
					w.WriteHeader(401)
					return
				}
				decision(w)
			})
			svc.AuthType = "oauth_client_credentials"
			svc.OAuth = &ServiceOAuth{TokenURL: strings.TrimSuffix(svc.URL, "/decision") + "/token", ClientID: "client", ClientSecret: "secret", Scopes: []string{"approval"}, Audience: "decision", AuthStyle: style}
			var wg sync.WaitGroup
			for range 8 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if _, err := Call(context.Background(), svc, Request{Phase: "check"}); err != nil {
						t.Error(err)
					}
				}()
			}
			wg.Wait()
			if fetches.Load() != 1 {
				t.Fatalf("token fetches %d", fetches.Load())
			}
			reject.Store(true)
			before := calls.Load()
			if _, err := Call(context.Background(), svc, Request{Phase: "consume"}); err == nil {
				t.Fatal("401 accepted")
			}
			if calls.Load() != before+1 {
				t.Fatal("consume replayed")
			}
			reject.Store(false)
			if _, err := Call(context.Background(), svc, Request{}); err != nil {
				t.Fatal(err)
			}
			if fetches.Load() != 2 {
				t.Fatal("rejected token not invalidated")
			}
			rotated := *svc.OAuth
			rotated.ClientSecret = "rotated"
			svc.OAuth = &rotated
			if _, err := Call(context.Background(), svc, Request{}); err != nil {
				t.Fatal(err)
			}
			if fetches.Load() != 3 {
				t.Fatal("credential rotation reused token")
			}
		})
	}
}
func TestServiceTimeoutIncludesOAuth(t *testing.T) {
	var decisions atomic.Int32
	svc := serviceFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			select {
			case <-r.Context().Done():
			case <-time.After(1500 * time.Millisecond):
			}
			return
		}
		decisions.Add(1)
		decision(w)
	})
	svc.AuthType = "oauth_client_credentials"
	svc.TimeoutSeconds = 1
	svc.OAuth = &ServiceOAuth{TokenURL: strings.TrimSuffix(svc.URL, "/decision") + "/token", ClientID: "client", ClientSecret: "secret"}
	start := time.Now()
	if _, err := Call(context.Background(), svc, Request{}); err == nil {
		t.Fatal("timeout accepted")
	}
	if time.Since(start) > 2*time.Second || decisions.Load() != 0 {
		t.Fatal("OAuth escaped phase deadline")
	}
}
func TestServiceLongerTimeoutAndCancellation(t *testing.T) {
	svc := serviceFixture(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2100 * time.Millisecond):
			decision(w)
		case <-r.Context().Done():
		}
	})
	svc.AuthType = "none"
	svc.TimeoutSeconds = 3
	if _, err := Call(context.Background(), svc, Request{}); err != nil {
		t.Fatal("configured timeout still capped at two seconds", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := Call(ctx, svc, Request{}); err == nil {
		t.Fatal("cancellation ignored")
	}
	if time.Since(start) > time.Second {
		t.Fatal("caller deadline ignored")
	}
}
func TestServiceCredentialRetentionAndRedaction(t *testing.T) {
	old := Service{URL: "https://service.test/check", AuthType: "oauth_client_credentials", OAuth: &ServiceOAuth{TokenURL: "https://idp.test/token", ClientID: "client", ClientSecret: "secret"}}
	redacted := old.Redacted(true)
	if redacted.OAuth.ClientSecret != "" || !redacted.OAuth.HasClientSecret || old.OAuth.ClientSecret != "secret" {
		t.Fatal("secret leak or source mutation")
	}
	redacted.OAuth.AuthStyle = "client_secret_basic" // Explicit default is the same authentication method.
	retained, err := redacted.RetainCredentials(old)
	if err != nil || retained.OAuth.ClientSecret != "secret" {
		t.Fatal(err)
	}
	for _, change := range []func(*Service){func(s *Service) { s.URL = "https://other.test" }, func(s *Service) { s.CAPEM = "different" }, func(s *Service) { s.OAuth.TokenURL = "https://other.test/token" }, func(s *Service) { s.OAuth.ClientID = "other" }, func(s *Service) { s.OAuth.AuthStyle = "client_secret_post" }} {
		next := old.Redacted(true)
		change(&next)
		if _, err := next.RetainCredentials(old); err == nil {
			t.Fatal("retained credentials across trust boundary")
		}
	}
	headers := Service{URL: old.URL, AuthType: "headers", Headers: map[string]string{"X-Key": "private"}}
	visible := headers.Redacted(true)
	if visible.Headers["X-Key"] != "" || !visible.HasHeaders["X-Key"] || headers.Headers["X-Key"] != "private" {
		t.Fatal("header leak/mutation")
	}
	if restored, err := visible.RetainCredentials(headers); err != nil || restored.Headers["X-Key"] != "private" {
		t.Fatal(err)
	}
	none := Service{URL: old.URL, AuthType: "none"}
	if _, err := none.RetainCredentials(old); err != nil {
		t.Fatal(err)
	}
	for _, svc := range []Service{{AuthType: "headers", Headers: map[string]string{"X-Key": "a", "x-key": "b"}}, {AuthType: "headers", Headers: map[string]string{"Host": "evil"}}, {AuthType: "headers", Headers: map[string]string{"X-Key": "a\r\nb"}}, {AuthType: "none", Token: "hidden"}, {TimeoutSeconds: 61}, {AuthType: "unknown"}} {
		if svc.ValidateConnection() == nil {
			t.Fatalf("invalid configuration accepted: %+v", svc)
		}
	}
}

func TestServiceOAuthEgressRedirectAndErrorRedaction(t *testing.T) {
	var destinationCalls atomic.Int32
	svc := serviceFixture(t, func(w http.ResponseWriter, r *http.Request) {
		destinationCalls.Add(1)
		if r.URL.Path == "/token" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":"secret-must-not-escape"}`)
			return
		}
		http.Redirect(w, r, "https://forbidden.example/token", 302)
	})
	svc.AuthType = "oauth_client_credentials"
	svc.OAuth = &ServiceOAuth{TokenURL: strings.TrimSuffix(svc.URL, "/decision") + "/token", ClientID: "client", ClientSecret: "secret-must-not-escape"}
	_, err := Call(context.Background(), svc, Request{})
	if err == nil || strings.Contains(err.Error(), "secret-must-not-escape") {
		t.Fatal("token failure leaked secret or succeeded", err)
	}
	svc.OAuth.TokenURL = strings.TrimSuffix(svc.URL, "/decision") + "/redirect"
	if _, err := Call(context.Background(), svc, Request{}); err == nil {
		t.Fatal("redirect accepted")
	}
	before := destinationCalls.Load()
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "")
	// Clients capture an immutable startup egress policy; use a new transport.
	svc.CAPEM += "\n"
	svc.OAuth.TokenURL = strings.TrimSuffix(svc.URL, "/decision") + "/blocked"
	if _, err := Call(context.Background(), svc, Request{}); err == nil {
		t.Fatal("private token destination allowed")
	}
	if destinationCalls.Load() != before {
		t.Fatal("blocked destination reached")
	}
}
func TestServiceTokenWaitHonorsCancellation(t *testing.T) {
	source := &serviceToken{gate: make(chan struct{}, 1)}
	source.gate <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := source.get(ctx, Service{}, nil); err == nil {
		t.Fatal("cache wait ignored cancellation")
	}
	if time.Since(start) > time.Second {
		t.Fatal("unbounded cache wait")
	}
}
