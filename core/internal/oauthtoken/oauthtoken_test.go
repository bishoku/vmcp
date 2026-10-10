package oauthtoken

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// tokenServer issues "tok-N" with the given lifetime and records the form it received.
func tokenServer(t *testing.T, expiresIn int, status int) (*httptest.Server, *atomic.Int32, *sync.Map) {
	t.Helper()
	var n atomic.Int32
	forms := &sync.Map{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		forms.Store("last", r.PostForm)
		if u, p, ok := r.BasicAuth(); ok {
			forms.Store("basic", u+":"+p)
		}
		if status != 200 {
			http.Error(w, `{"error":"invalid_client","error_description":"bad secret s3cret"}`, status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":"tok-%d","token_type":"Bearer","expires_in":%d}`, n.Add(1), expiresIn)
	}))
	t.Cleanup(srv.Close)
	return srv, &n, forms
}

func TestSingleFetchForConcurrentCallers(t *testing.T) {
	srv, n, _ := tokenServer(t, 3600, 200)
	s := New(Config{TokenURL: srv.URL, ClientID: "c", ClientSecret: "s3cret"}, nil)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if tok, err := s.Token(context.Background()); err != nil || tok != "tok-1" {
				t.Errorf("tok=%q err=%v", tok, err)
			}
		}()
	}
	wg.Wait()
	if n.Load() != 1 || s.Fetches() != 1 {
		t.Fatalf("token endpoint called %d times, want 1", n.Load())
	}
}

func TestRefreshBeforeExpiry(t *testing.T) {
	srv, n, _ := tokenServer(t, 3600, 200)
	s := New(Config{TokenURL: srv.URL, ClientID: "c", ClientSecret: "x"}, nil)
	now := time.Now()
	s.now = func() time.Time { return now }
	first, _ := s.Token(context.Background())
	now = now.Add(3600*time.Second - 25*time.Second) // past the 30-40s early-refresh point, before expiry
	second, _ := s.Token(context.Background())
	if first == second || n.Load() != 2 {
		t.Fatalf("expected a refresh before expiry: %q %q (%d fetches)", first, second, n.Load())
	}
	now = now.Add(time.Second)
	if third, _ := s.Token(context.Background()); third != second {
		t.Fatal("token should be cached again after refresh")
	}
}

func TestScopesAudienceAndFailure(t *testing.T) {
	srv, _, forms := tokenServer(t, 60, 200)
	s := New(Config{TokenURL: srv.URL, ClientID: "c", ClientSecret: "x", Scopes: []string{"a", "b"}, Audience: "https://api"}, nil)
	if _, err := s.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	f, _ := forms.Load("last")
	form := f.(url.Values)
	if form["grant_type"][0] != "client_credentials" || form["scope"][0] != "a b" || form["audience"][0] != "https://api" {
		t.Fatalf("form: %v", form)
	}

	bad, _, _ := tokenServer(t, 60, 401)
	s = New(Config{TokenURL: bad.URL, ClientID: "c", ClientSecret: "s3cret"}, nil)
	_, err := s.Token(context.Background())
	if !IsTokenError(err) {
		t.Fatalf("want token error, got %v", err)
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("secret leaked in error: %v", err)
	}
}

func TestTransportInjectsAndRetriesOnce(t *testing.T) {
	srv, n, _ := tokenServer(t, 3600, 200)
	var calls atomic.Int32
	var seen []string
	var mu sync.Mutex
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization")+"|"+string(b))
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer tok-2" { // first token is "revoked"
			http.Error(w, "no", 401)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(up.Close)

	s := New(Config{TokenURL: srv.URL, ClientID: "c", ClientSecret: "x"}, nil)
	hc := &http.Client{Transport: Transport(nil, s)}
	resp, err := hc.Post(up.URL, "text/plain", strings.NewReader("payload"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || calls.Load() != 2 || n.Load() != 2 {
		t.Fatalf("status=%d upstream calls=%d token fetches=%d", resp.StatusCode, calls.Load(), n.Load())
	}
	if seen[1] != "Bearer tok-2|payload" {
		t.Fatalf("body must be replayed with the new token: %v", seen)
	}

	// An upstream that always says 401 is retried exactly once, not forever.
	calls.Store(0)
	always := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "no", 401)
	}))
	t.Cleanup(always.Close)
	resp, err = hc.Get(always.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 || calls.Load() != 2 {
		t.Fatalf("status=%d calls=%d", resp.StatusCode, calls.Load())
	}
}

func TestForSharesSourcePerConfig(t *testing.T) {
	a := For(Config{TokenURL: "http://x/t", ClientID: "c", ClientSecret: "1"})
	if For(Config{TokenURL: "http://x/t", ClientID: "c", ClientSecret: "1"}) != a {
		t.Fatal("same config must share a source")
	}
	if For(Config{TokenURL: "http://x/t", ClientID: "c", ClientSecret: "2"}) == a {
		t.Fatal("rotated secret must get a fresh source")
	}
}

func TestTokenFetchDeniesUnapprovedDestination(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "")
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer srv.Close()
	source := New(Config{TokenURL: srv.URL, ClientID: "client", ClientSecret: "secret"}, nil)
	if _, err := source.Token(context.Background()); err == nil || !IsTokenError(err) {
		t.Fatalf("blocked token endpoint: %v", err)
	}
	if hits != 0 {
		t.Fatal("secret sent to blocked token endpoint")
	}
}

func TestRetainRetiresSupersededSources(t *testing.T) {
	t.Cleanup(func() { regMu.Lock(); defer regMu.Unlock(); retained = nil; registry = map[string]*Source{} })
	a := Config{TokenURL: "https://idp.example/token", ClientID: "a", ClientSecret: "old"}
	b := a
	b.ClientSecret = "new"
	old := For(a)
	current := For(b)
	Retain([]Config{b})
	if For(b) != current {
		t.Fatal("unchanged token source was discarded")
	}
	if For(a) == For(a) {
		t.Fatal("obsolete source was reintroduced into registry")
	}
	if For(a) == old {
		t.Fatal("superseded token source remained reusable")
	}
	Retain(nil)
}
