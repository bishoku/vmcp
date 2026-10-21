package upstream

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/callercontext"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/netpolicy"
	"github.com/bishoku/vmcp/core/internal/oauthtoken"
	"github.com/bishoku/vmcp/core/internal/tlsconf"
)

// Conn is a session with one upstream, regardless of transport.
type Conn interface {
	Initialize(ctx context.Context) (*mcp.InitializeResult, error)
	ListTools(ctx context.Context) ([]mcp.Tool, error)
	CallTool(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error)
	Close(ctx context.Context) error
}

// Open returns an unconnected Conn for the given transport ("streamable_http" or "sse").
func Open(transport, url string, headers map[string]string, tl tlsconf.Settings) (Conn, error) {
	return OpenAuth(transport, url, headers, tl, nil)
}

// OpenAuth is Open with an optional OAuth client: every request then carries a cached
// client_credentials bearer token (and is replayed once if the upstream answers 401).
func OpenAuth(transport, url string, headers map[string]string, tl tlsconf.Settings, oc *oauthtoken.Config) (Conn, error) {
	hc, err := tl.Client()
	if err != nil {
		return nil, err
	}
	hc = netpolicy.Client(hc)
	if oc != nil {
		// tl.Client() hands out a shared client: wrap its transport in a new one, never mutate it.
		var base http.RoundTripper
		if hc != nil {
			base = hc.Transport
		}
		wrapped := *hc
		wrapped.Transport = oauthtoken.Transport(base, oauthtoken.For(*oc))
		hc = &wrapped
	}
	switch transport {
	case "streamable_http", "":
		c := NewClient(url, headers, nil)
		c.http = hc
		return c, nil
	case "sse":
		c := NewSSEClient(url, headers, nil)
		c.http = hc
		return c, nil
	}
	return nil, fmt.Errorf("unsupported transport %q", transport)
}

// SSEClient speaks the legacy HTTP+SSE transport (MCP 2024-11-05): the client opens
// a long-lived GET event stream, learns the POST endpoint from an "endpoint" event,
// then posts requests there and receives every response on the stream.
type SSEClient struct {
	capabilities mcp.ServerCapabilities
	loggingMu    sync.Mutex
	loggingLevel string
	url          string
	headers      map[string]string
	http         *http.Client
	nextID       atomic.Int64

	// OnServerMessage receives server-initiated messages. Optional.
	OnServerMessage func(*mcp.Message)

	mu            sync.Mutex
	endpoint      string
	pending       map[string]chan *mcp.Message
	relayContexts map[string]context.Context
	requestSlots  chan struct{}
	closed        chan struct{} // closed when the event stream ends
	cancel        context.CancelFunc
}

// NewSSEClient creates a legacy SSE client; the stream opens on Initialize.
func NewSSEClient(url string, headers map[string]string, hc *http.Client) *SSEClient {
	if hc == nil {
		hc = &http.Client{}
	}
	return &SSEClient{url: url, headers: headers, http: netpolicy.Client(hc), pending: map[string]chan *mcp.Message{}, requestSlots: make(chan struct{}, 32)}
}

// Initialize opens the event stream, then performs the MCP handshake.
func (c *SSEClient) Initialize(ctx context.Context) (*mcp.InitializeResult, error) {
	if err := c.connect(ctx); err != nil {
		return nil, err
	}
	handshakeCtx := WithRelay(ctx, Relay{ClientCapabilities: ClientCapabilities(ctx)})
	raw, err := c.call(handshakeCtx, mcp.MethodInitialize, mcp.InitializeParams{
		ProtocolVersion: mcp.SupportedVersions[0],
		Capabilities:    ClientCapabilities(ctx),
		ClientInfo:      mcp.Implementation{Name: "vmcp-gateway", Version: "0.1.0"},
	})
	if err != nil {
		c.shutdown()
		return nil, err
	}
	res, err := mcp.ValidateInitializeResult(raw)
	if err != nil {
		c.shutdown()
		return nil, fmt.Errorf("decode initialize result: %w", err)
	}

	n, _ := mcp.NewRequest(mcp.ID{}, mcp.MethodInitialized, nil)
	if err := c.post(ctx, n); err != nil {
		c.shutdown()
		return nil, fmt.Errorf("send initialized: %w", err)
	}
	return res, nil
}

// ListTools returns every tool, following pagination cursors.
func (c *SSEClient) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	return listTools(ctx, c.call)
}

// CallTool invokes a tool and returns the raw result object.
func (c *SSEClient) CallTool(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	return c.call(ctx, mcp.MethodToolsCall, mcp.CallToolParams{Name: name, Arguments: args, Meta: relayFrom(ctx).Meta})
}

// Close ends the event stream.
func (c *SSEClient) Close(context.Context) error {
	c.shutdown()
	return nil
}

func (c *SSEClient) shutdown() {
	c.mu.Lock()
	cancel := c.cancel
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (c *SSEClient) applyHeaders(req *http.Request) {
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
}

func (c *SSEClient) connect(ctx context.Context) error {
	// The stream outlives the request that opened it, so it gets its own context.
	sctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(sctx, http.MethodGet, c.url, nil)
	if err != nil {
		cancel()
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	c.applyHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		cancel()
		return fmt.Errorf("open event stream: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		cancel()
		return fmt.Errorf("open event stream returned %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	closed := make(chan struct{})
	ready := make(chan error, 1)
	c.mu.Lock()
	c.cancel, c.closed = cancel, closed
	c.mu.Unlock()
	go c.readLoop(resp.Body, ready, closed)

	select {
	case err := <-ready:
		if err != nil {
			cancel()
			return err
		}
		return nil
	case <-ctx.Done():
		cancel()
		return ctx.Err()
	}
}

func (c *SSEClient) readLoop(body io.ReadCloser, ready chan<- error, closed chan struct{}) {
	defer body.Close()
	defer close(closed)
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64<<10), maxSSEEventBytes)

	var event string
	var data []string
	frameBytes := 0
	gotEndpoint := false
	dispatch := func() {
		defer func() { event, data, frameBytes = "", data[:0], 0 }()
		if len(data) == 0 {
			return
		}
		payload := strings.Join(data, "\n")
		if event == "endpoint" {
			base, err := url.Parse(c.url)
			ref, err2 := url.Parse(strings.TrimSpace(payload))
			if err != nil || err2 != nil {
				select {
				case ready <- fmt.Errorf("invalid endpoint event"):
				default:
				}
				return
			}
			endpoint := base.ResolveReference(ref)
			if netpolicy.ValidateURL(endpoint.String()) != nil || !sameOrigin(base, endpoint) {
				select {
				case ready <- fmt.Errorf("SSE endpoint must have the same origin as the stream"):
				default:
				}
				return
			}
			if gotEndpoint {
				return
			}
			c.mu.Lock()
			c.endpoint = endpoint.String()
			c.mu.Unlock()
			if !gotEndpoint {
				gotEndpoint = true
				ready <- nil
			}
			return
		}
		m, err := mcp.Decode([]byte(payload))
		if err != nil {
			return
		}
		if m.IsResponse() {
			c.mu.Lock()
			ch := c.pending[m.ID.String()]
			delete(c.pending, m.ID.String())
			c.mu.Unlock()
			if ch != nil {
				ch <- m
			}
			return
		}
		c.mu.Lock()
		messageCtx := selectRelay(c.relayContexts, m)
		c.mu.Unlock()
		if m.IsRequest() {
			select {
			case c.requestSlots <- struct{}{}:
				go func() {
					defer func() { <-c.requestSlots }()
					_ = handleMessage(messageCtx, m, c.Send, c.OnServerMessage)
				}()
			default:
			}
			return
		}
		_ = handleMessage(messageCtx, m, c.Send, c.OnServerMessage)
	}
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			dispatch()
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			frameBytes += len(line)
			if frameBytes > maxSSEEventBytes {
				return
			}
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if !gotEndpoint {
		ready <- fmt.Errorf("event stream ended before endpoint event")
	}
}

func (c *SSEClient) post(ctx context.Context, msg *mcp.Message) error {
	c.mu.Lock()
	endpoint := c.endpoint
	c.mu.Unlock()
	if endpoint == "" {
		return ErrSessionExpired
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.applyHeaders(req)
	if msg.Method == mcp.MethodToolsCall {
		callercontext.ApplyHeaders(ctx, req)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("post %s: %w", msg.Method, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("upstream %s returned %d: %s", msg.Method, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func (c *SSEClient) call(ctx context.Context, method string, params any) (auditResult json.RawMessage, auditErr error) {
	complete, auditStartErr := audit.Observe(ctx, method, params)
	if auditStartErr != nil {
		return nil, auditStartErr
	}
	defer func() {
		if err := complete(auditResult, auditErr); err != nil {
			auditResult = nil
			auditErr = err
		}
	}()

	id := mcp.NewIntID(c.nextID.Add(1))
	defer cancelRequest(ctx, method, id, c.Send)
	req, err := mcp.NewRequest(id, method, params)
	if err != nil {
		return nil, err
	}
	ch := make(chan *mcp.Message, 1)
	c.mu.Lock()
	closed := c.closed
	c.pending[id.String()] = ch
	if c.relayContexts == nil {
		c.relayContexts = map[string]context.Context{}
	}
	c.relayContexts[id.String()] = ctx
	defer func() { c.mu.Lock(); delete(c.relayContexts, id.String()); c.mu.Unlock() }()
	c.mu.Unlock()
	drop := func() {
		c.mu.Lock()
		delete(c.pending, id.String())
		c.mu.Unlock()
	}
	if closed == nil {
		drop()
		return nil, ErrSessionExpired
	}
	select {
	case <-closed:
		drop()
		return nil, ErrSessionExpired
	default:
	}
	if err := c.post(ctx, req); err != nil {
		drop()
		return nil, err
	}
	select {
	case m := <-ch:
		if m.Error != nil {
			return nil, m.Error
		}
		return m.Result, nil
	case <-closed:
		drop()
		return nil, ErrSessionExpired
	case <-ctx.Done():
		drop()
		return nil, ctx.Err()
	}
}

func sameOrigin(a, b *url.URL) bool {
	port := func(u *url.URL) string {
		if u.Port() != "" {
			return u.Port()
		}
		if u.Scheme == "https" {
			return "443"
		}
		return "80"
	}
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Hostname(), b.Hostname()) && port(a) == port(b)
}
