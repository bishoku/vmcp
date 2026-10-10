package virtual

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/store"
)

type capture struct {
	mu sync.Mutex
	ev []audit.Event
}

func (c *capture) Record(e audit.Event) { c.mu.Lock(); c.ev = append(c.ev, e); c.mu.Unlock() }

var lastCapture *capture

func authSetup(t *testing.T, trusted ...netip.Prefix) *httptest.Server {
	t.Helper()
	lastCapture = &capture{}
	past, future := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	key := func(slug string, allowed []netip.Prefix, exp *time.Time, servers ...string) config.AgentKey {
		k := config.AgentKey{Slug: slug, Allowed: allowed, ExpiresAt: exp, Servers: map[string]bool{}}
		for _, s := range servers {
			k.Servers[s] = true
		}
		return k
	}
	snap := &config.Snapshot{
		VirtualServers: map[string]config.VirtualServer{
			"locked": {Slug: "locked", AuthRequired: true},
			"open":   {Slug: "open"},
		},
		AgentKeys: map[string]config.AgentKey{
			string(store.HashKey("vmcp_good")):     key("good", nil, nil, "locked"),
			string(store.HashKey("vmcp_other")):    key("other", nil, nil, "locked"),
			string(store.HashKey("vmcp_expired")):  key("expired", nil, &past, "locked"),
			string(store.HashKey("vmcp_future")):   key("future", nil, &future, "locked"),
			string(store.HashKey("vmcp_noserver")): key("noserver", nil, nil, "open"),
			string(store.HashKey("vmcp_local")):    key("local", []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}, nil, "locked"),
			string(store.HashKey("vmcp_lan")):      key("lan", []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, nil, "locked"),
		},
	}
	mux := http.NewServeMux()
	(&Handler{Snap: staticSnap{snap}, Sessions: NewManager(), Log: slog.New(slog.NewTextHandler(io.Discard, nil)), TrustedProxies: trusted, Audit: lastCapture}).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

const initBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`

func post(t *testing.T, url, body string, hdr map[string]string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest("POST", url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func TestAgentAuth(t *testing.T) {
	srv := authSetup(t)
	locked := srv.URL + "/mcp/v1/virtual/locked"
	bearer := func(k string) map[string]string { return map[string]string{"Authorization": "Bearer " + k} }

	cases := []struct {
		name string
		url  string
		hdr  map[string]string
		want int
	}{
		{"no key", locked, nil, 401},
		{"unknown key", locked, bearer("vmcp_nope"), 401},
		{"expired key", locked, bearer("vmcp_expired"), 401},
		{"valid key with future expiry", locked, bearer("vmcp_future"), 200},
		{"key not assigned to server", locked, bearer("vmcp_noserver"), 403},
		{"ip allowed", locked, bearer("vmcp_local"), 200},
		{"ip not allowed", locked, bearer("vmcp_lan"), 403},
		{"spoofed XFF ignored without trusted proxy", locked, map[string]string{"Authorization": "Bearer vmcp_lan", "X-Forwarded-For": "10.1.1.1"}, 403},
		{"open server needs no key", srv.URL + "/mcp/v1/virtual/open", nil, 200},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp, body := post(t, c.url, initBody, c.hdr)
			if resp.StatusCode != c.want {
				t.Fatalf("status=%d want %d (%s)", resp.StatusCode, c.want, body)
			}
			if c.want == 401 && resp.Header.Get("WWW-Authenticate") == "" {
				t.Fatal("missing WWW-Authenticate")
			}
		})
	}

	t.Run("session is bound to the key that opened it", func(t *testing.T) {
		resp, _ := post(t, locked, initBody, bearer("vmcp_good"))
		sid := resp.Header.Get("Mcp-Session-Id")
		list := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
		if r, _ := post(t, locked, list, map[string]string{"Authorization": "Bearer vmcp_good", "Mcp-Session-Id": sid}); r.StatusCode != 200 {
			t.Fatalf("same key: %d", r.StatusCode)
		}
		if r, _ := post(t, locked, list, map[string]string{"Authorization": "Bearer vmcp_other", "Mcp-Session-Id": sid}); r.StatusCode != 404 {
			t.Fatalf("other key: %d, want 404", r.StatusCode)
		}
		if r, _ := post(t, locked, list, map[string]string{"Mcp-Session-Id": sid}); r.StatusCode != 401 {
			t.Fatalf("no key on live session: %d, want 401", r.StatusCode)
		}
	})
}

func TestAgentAuthTrustedProxy(t *testing.T) {
	srv := authSetup(t, netip.MustParsePrefix("127.0.0.1/32"))
	locked := srv.URL + "/mcp/v1/virtual/locked"
	h := func(xff string) map[string]string {
		return map[string]string{"Authorization": "Bearer vmcp_lan", "X-Forwarded-For": xff}
	}
	if r, _ := post(t, locked, initBody, h("10.1.1.1")); r.StatusCode != 200 {
		t.Fatalf("trusted proxy, allowed client: %d", r.StatusCode)
	}
	if r, _ := post(t, locked, initBody, h("8.8.8.8")); r.StatusCode != 403 {
		t.Fatalf("trusted proxy, disallowed client: %d", r.StatusCode)
	}
	// A client-supplied left-most value cannot override the proxy-appended one.
	if r, _ := post(t, locked, initBody, h("10.1.1.1, 8.8.8.8")); r.StatusCode != 403 {
		t.Fatalf("spoofed prefix: %d", r.StatusCode)
	}
}

func TestDeniedRequestsAreAudited(t *testing.T) {
	srv := authSetup(t)
	post(t, srv.URL+"/mcp/v1/virtual/locked", initBody, map[string]string{"Authorization": "Bearer vmcp_nope"})
	lastCapture.mu.Lock()
	defer lastCapture.mu.Unlock()
	if len(lastCapture.ev) != 1 || lastCapture.ev[0].Status != "denied" || lastCapture.ev[0].VirtualServer != "locked" {
		t.Fatalf("events=%+v", lastCapture.ev)
	}
	if strings.Contains(lastCapture.ev[0].Reason, "vmcp_nope") {
		t.Fatal("presented key leaked into audit reason")
	}
}
