// Package netpolicy enforces operator-owned network boundaries for MCP and token traffic.
package netpolicy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Policy struct {
	prefixes []netip.Prefix
	hosts    map[string]bool
	lookup   func(context.Context, string, string) ([]netip.Addr, error)
	dial     func(context.Context, string, string) (net.Conn, error)
}

// FromEnv captures an immutable policy. Public unicast destinations are allowed
// unless a host allowlist is set; private/loopback ranges always require explicit CIDRs.
func FromEnv() (*Policy, error) {
	p := &Policy{hosts: map[string]bool{}, lookup: net.DefaultResolver.LookupNetIP}
	d := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	p.dial = d.DialContext
	for _, entry := range split(os.Getenv("VMCP_EGRESS_ALLOW_CIDRS")) {
		prefix, err := netip.ParsePrefix(entry)
		if err != nil || prefix.Addr().Is4In6() {
			return nil, fmt.Errorf("%w: invalid VMCP_EGRESS_ALLOW_CIDRS entry %q", ErrInvalidPolicy, entry)
		}
		p.prefixes = append(p.prefixes, prefix.Masked())
	}
	for _, host := range split(os.Getenv("VMCP_EGRESS_ALLOW_HOSTS")) {
		host = strings.ToLower(host)
		if ip, err := netip.ParseAddr(host); err == nil && ip.Zone() == "" {
			p.hosts[ip.Unmap().String()] = true
			continue
		}
		if strings.ContainsAny(host, "/:@*% ") || strings.HasSuffix(host, ".") {
			return nil, fmt.Errorf("%w: invalid VMCP_EGRESS_ALLOW_HOSTS entry %q", ErrInvalidPolicy, host)
		}
		p.hosts[host] = true
	}
	return p, nil
}
func split(s string) []string {
	var out []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

// ValidateURL performs syntactic validation without DNS or network side effects.
func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" || strings.ContainsAny(u.Hostname(), "% \\@") {
		return errors.New("must be an absolute http(s) URL without credentials, fragments or address zones")
	}
	if strings.HasSuffix(u.Host, ":") {
		return errors.New("URL port is empty")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return errors.New("invalid URL port")
		}
	}
	return nil
}
func (p *Policy) checkHost(host string) error {
	if ip, err := netip.ParseAddr(host); err == nil {
		host = ip.Unmap().String()
	}
	if len(p.hosts) > 0 && !p.hosts[strings.ToLower(host)] {
		return ErrHostNotAllowed
	}
	return nil
}
func (p *Policy) checkAddr(ip netip.Addr) error {
	ip = ip.Unmap()
	if ip.Is6() && netip.MustParsePrefix("::/96").Contains(ip) && !ip.IsLoopback() {
		return ErrDestinationProhibited
	}
	// Never permit metadata, unspecified, multicast, broadcast or reserved ranges,
	// even if an operator accidentally supplies an overly broad allow CIDR.
	for _, prefix := range prohibited {
		if prefix.Contains(ip) {
			return ErrDestinationProhibited
		}
	}
	if (!ip.IsGlobalUnicast() && !ip.IsLoopback()) || ip.Zone() != "" || ip.IsLinkLocalUnicast() || ip == netip.MustParseAddr("100.100.100.200") || ip == netip.MustParseAddr("fd00:ec2::254") || ip == netip.MustParseAddr("168.63.129.16") || (ip.Is4() && (netip.MustParsePrefix("0.0.0.0/8").Contains(ip) || netip.MustParsePrefix("240.0.0.0/4").Contains(ip))) {
		return ErrDestinationProhibited
	}
	restricted := ip.IsPrivate() || ip.IsLoopback() || netip.MustParsePrefix("100.64.0.0/10").Contains(ip)
	if restricted {
		for _, prefix := range p.prefixes {
			if prefix.Contains(ip) {
				return nil
			}
		}
		return ErrPrivateDestination
	}
	return nil
}
func (p *Policy) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if err = p.checkHost(host); err != nil {
		return nil, err
	}
	var ips []netip.Addr
	if ip, e := netip.ParseAddr(host); e == nil {
		ips = []netip.Addr{ip}
	} else {
		ips, err = p.lookup(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
	}
	if len(ips) == 0 {
		return nil, ErrNoAddresses
	}
	// Validate every answer before dialing any, then dial the checked IP directly.
	for _, ip := range ips {
		if err = p.checkAddr(ip); err != nil {
			return nil, err
		}
	}
	for _, ip := range ips {
		var conn net.Conn
		conn, err = p.dial(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
		if err == nil {
			return conn, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, err
}

type guarded struct {
	base   *http.Transport
	policy *Policy
}

func (g *guarded) RoundTrip(r *http.Request) (*http.Response, error) {
	if err := ValidateURL(r.URL.String()); err != nil {
		return nil, err
	}
	if err := g.policy.checkHost(r.URL.Hostname()); err != nil {
		return nil, err
	}
	return g.base.RoundTrip(r)
}

type reject struct{ err error }

func (r reject) RoundTrip(*http.Request) (*http.Response, error) { return nil, r.err }

var cache = struct {
	sync.Mutex
	transports map[string]map[*http.Transport]*guarded
}{transports: map[string]map[*http.Transport]*guarded{}}

// Client clones caller settings, preserving TLS verification and connection pools.
// Environment proxies and all redirects are deliberately disabled. Custom
// transports are rejected because their dialing/proxy behavior cannot be enforced.
func Client(base *http.Client) *http.Client {
	var client http.Client
	if base != nil {
		client = *base
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("egress redirects are disabled") }
	if _, ok := client.Transport.(*guarded); ok {
		return &client
	}
	p, err := FromEnv()
	if err != nil {
		client.Transport = reject{err}
		return &client
	}
	tr, ok := client.Transport.(*http.Transport)
	if client.Transport == nil {
		tr, ok = http.DefaultTransport.(*http.Transport)
	}
	if !ok {
		client.Transport = reject{ErrUnsupportedTransport}
		return &client
	}
	key := os.Getenv("VMCP_EGRESS_ALLOW_CIDRS") + "\x00" + os.Getenv("VMCP_EGRESS_ALLOW_HOSTS")
	cache.Lock()
	byBase := cache.transports[key]
	if byBase == nil {
		byBase = map[*http.Transport]*guarded{}
		cache.transports[key] = byBase
	}
	g := byBase[tr]
	if g == nil {
		clone := tr.Clone()
		if clone.MaxIdleConns < 2048 {
			clone.MaxIdleConns = 2048
		}
		if clone.MaxIdleConnsPerHost < 128 {
			clone.MaxIdleConnsPerHost = 128
		}
		if clone.IdleConnTimeout == 0 {
			clone.IdleConnTimeout = 90 * time.Second
		}
		clone.Proxy = nil
		clone.DialContext = p.DialContext
		clone.DialTLSContext = nil
		clone.DialTLS = nil
		g = &guarded{clone, p}
		byBase[tr] = g
	}
	cache.Unlock()
	client.Transport = g
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("egress redirects are disabled") }
	return &client
}

// Transition ranges can route embedded IPv4 destinations around address checks.
var prohibited = []netip.Prefix{
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("64:ff9b::/96"), netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("2001::/32"),
}
