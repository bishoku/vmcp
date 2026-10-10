package netpolicy

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"sync/atomic"
	"testing"
)

func policy(t *testing.T, cidrs, hosts string) *Policy {
	t.Helper()
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", cidrs)
	t.Setenv("VMCP_EGRESS_ALLOW_HOSTS", hosts)
	p, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func TestAddressPolicy(t *testing.T) {
	p := policy(t, "", "")
	for _, raw := range []string{"127.0.0.1", "::1", "10.0.0.1", "172.16.0.1", "192.168.0.1", "fc00::1", "169.254.169.254", "fe80::1", "::ffff:127.0.0.1", "::ffff:169.254.169.254", "::127.0.0.1", "0.0.0.0", "224.0.0.1", "255.255.255.255", "100.100.100.200", "fd00:ec2::254", "168.63.129.16", "64:ff9b::a00:1", "2002:7f00:1::"} {
		t.Run(raw, func(t *testing.T) {
			if p.checkAddr(netip.MustParseAddr(raw)) == nil {
				t.Fatal("unsafe address allowed")
			}
		})
	}
	for _, raw := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		if err := p.checkAddr(netip.MustParseAddr(raw)); err != nil {
			t.Fatalf("public %s: %v", raw, err)
		}
	}
	p = policy(t, "10.2.0.0/16,127.0.0.0/8,::1/128", "")
	for _, raw := range []string{"10.2.3.4", "127.0.0.1", "::ffff:127.0.0.1", "::1"} {
		if err := p.checkAddr(netip.MustParseAddr(raw)); err != nil {
			t.Fatalf("approved %s: %v", raw, err)
		}
	}
	if p.checkAddr(netip.MustParseAddr("10.3.0.1")) == nil {
		t.Fatal("unapproved subnet allowed")
	}
	p = policy(t, "0.0.0.0/0,::/0", "")
	for _, raw := range []string{"169.254.169.254", "100.100.100.200", "fd00:ec2::254", "64:ff9b::7f00:1", "224.0.0.1", "0.0.0.0"} {
		if p.checkAddr(netip.MustParseAddr(raw)) == nil {
			t.Fatalf("allow CIDR overrode hard denial: %s", raw)
		}
	}
}
func TestDNSRebindingAndMixedAnswers(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(map[bool]string{true: "mixed answers", false: "rebinding"}[mixed], func(t *testing.T) {
			p := policy(t, "", "mcp.example")
			lookups, dials := 0, 0
			p.lookup = func(context.Context, string, string) ([]netip.Addr, error) {
				lookups++
				if mixed {
					return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("127.0.0.1")}, nil
				}
				if lookups == 1 {
					return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
				}
				return []netip.Addr{netip.MustParseAddr("169.254.169.254")}, nil
			}
			p.dial = func(_ context.Context, _, address string) (net.Conn, error) {
				dials++
				if address != "8.8.8.8:443" {
					t.Fatalf("did not pin resolved IP: %s", address)
				}
				a, b := net.Pipe()
				b.Close()
				return a, nil
			}
			conn, err := p.DialContext(context.Background(), "tcp", "mcp.example:443")
			if mixed {
				if err == nil || dials != 0 {
					t.Fatal("dialed before validating all answers")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			conn.Close()
			if _, err = p.DialContext(context.Background(), "tcp", "mcp.example:443"); err == nil || dials != 1 {
				t.Fatal("rebound address was dialed")
			}
			if _, err = p.DialContext(context.Background(), "tcp", "other.example:443"); err == nil || lookups != 2 {
				t.Fatal("host allowlist was bypassed")
			}
		})
	}
}
func TestURLValidation(t *testing.T) {
	for _, raw := range []string{"file:///etc/passwd", "ftp://example.com", "http://user:secret@example.com", "http://example.com/#fragment", "http://[fe80::1%25eth0]/", "http://example.com:0", "http://example.com:99999", "//example.com", "http://example.com:"} {
		if ValidateURL(raw) == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	for _, raw := range []string{"https://example.com/mcp?x=1", "http://[::1]:8080/mcp"} {
		if err := ValidateURL(raw); err != nil {
			t.Errorf("rejected %s: %v", raw, err)
		}
	}
}
func TestRedirectAndProxyDoNotReceiveCredentials(t *testing.T) {
	policy(t, "127.0.0.0/8", "")
	var destinationHits, proxyHits atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { destinationHits.Add(1) }))
	defer destination.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { proxyHits.Add(1) }))
	defer proxy.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 307) }))
	defer source.Close()
	proxyURL, _ := url.Parse(proxy.URL)
	base := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	c := Client(base)
	req, _ := http.NewRequest("GET", source.URL, nil)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("X-Api-Key", "private")
	resp, err := c.Do(req)
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil || destinationHits.Load() != 0 || proxyHits.Load() != 0 {
		t.Fatalf("redirect/proxy bypass: %v %d %d", err, destinationHits.Load(), proxyHits.Load())
	}
}
func TestTLSVerificationPreserved(t *testing.T) {
	policy(t, "127.0.0.0/8", "")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok") }))
	defer srv.Close()
	resp, err := Client(srv.Client()).Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp, err = Client(nil).Get(srv.URL); err == nil {
		resp.Body.Close()
		t.Fatal("untrusted certificate accepted")
	}
	base := srv.Client()
	tr := base.Transport.(*http.Transport).Clone()
	tr.TLSClientConfig = tr.TLSClientConfig.Clone()
	tr.TLSClientConfig.ServerName = "wrong.example"
	base = &http.Client{Transport: tr}
	if resp, err = Client(base).Get(srv.URL); err == nil {
		resp.Body.Close()
		t.Fatal("hostname verification bypassed")
	}
}
func TestInvalidOperatorConfigurationFailsClosed(t *testing.T) {
	for _, cidr := range []string{"broken", "::ffff:127.0.0.1/128"} {
		t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", cidr)
		if _, err := FromEnv(); err == nil {
			t.Fatalf("invalid CIDR accepted: %s", cidr)
		}
		if _, err := Client(nil).Get("http://127.0.0.1"); err == nil {
			t.Fatal("bad config allowed a request")
		}
	}
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "")
	t.Setenv("VMCP_EGRESS_ALLOW_HOSTS", "*.example.com")
	if _, err := FromEnv(); err == nil {
		t.Fatal("wildcard host accepted")
	}
}
func TestExplicitHostDoesNotAuthorizePrivateIP(t *testing.T) {
	p := policy(t, "", "internal.example")
	p.lookup = func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("10.1.2.3")}, nil
	}
	p.dial = func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("dialed unauthorized private IP")
		return nil, errors.New("unreachable")
	}
	if _, err := p.DialContext(context.Background(), "tcp", "internal.example:80"); err == nil {
		t.Fatal("host grant bypassed CIDR policy")
	}
}
