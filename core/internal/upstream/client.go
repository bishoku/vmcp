// Package upstream talks to physical MCP servers on behalf of the gateway.
package upstream

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/callercontext"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/netpolicy"
)

// ErrSessionExpired means the upstream no longer knows our session (HTTP 404).
// Callers should re-initialize and retry.
var ErrSessionExpired = errors.New("upstream session expired")

const maxSSEEventBytes = 10 << 20

// Client is a Streamable HTTP MCP client bound to one upstream and one session.
type Client struct {
	preference       preferenceKey
	preferenceExpiry time.Time
	retired          atomic.Bool
	capabilities     mcp.ServerCapabilities
	loggingMu        sync.Mutex
	loggingLevel     string
	url              string
	headers          map[string]string
	http             *http.Client
	nextID           atomic.Int64

	// OnServerMessage receives server-initiated requests and notifications
	// (progress, logging, list_changed) seen on a response stream. Optional.
	OnServerMessage func(*mcp.Message)

	mu              sync.RWMutex
	sessionID       string
	protocolVersion string
}

// NewClient creates a client. headers are sent on every request (e.g. Authorization).
func NewClient(url string, headers map[string]string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 0} // per-call deadlines come from the context
	}
	return &Client{url: url, headers: headers, http: netpolicy.Client(hc)}
}

// SessionID returns the current upstream session id, if any.
func (c *Client) SessionID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionID
}

// Initialize performs the MCP handshake and records the session.
func (c *Client) Initialize(ctx context.Context) (*mcp.InitializeResult, error) {
	if mcp.ModernEnabled() {
		discovered, err := c.discover(ctx)
		if err != nil || discovered != nil {
			return discovered, err
		}
	}
	return c.initializeLegacy(ctx)
}

func (c *Client) initializeLegacy(ctx context.Context) (*mcp.InitializeResult, error) {
	params := mcp.InitializeParams{
		ProtocolVersion: mcp.ProtocolVersion20250618,
		Capabilities:    ClientCapabilities(ctx),
		ClientInfo:      mcp.Implementation{Name: "vmcp-gateway", Version: "0.1.0"},
	}
	handshakeCtx := WithRelay(ctx, Relay{ClientCapabilities: ClientCapabilities(ctx)})
	raw, err := c.Call(handshakeCtx, mcp.MethodInitialize, params)
	if err != nil {
		return nil, err
	}
	res, err := mcp.ValidateInitializeResult(raw)
	if err != nil {
		return nil, fmt.Errorf("decode initialize result: %w", err)
	}

	c.mu.Lock()
	c.protocolVersion = res.ProtocolVersion
	c.capabilities = res.Capabilities
	c.mu.Unlock()

	if err := c.Notify(ctx, mcp.MethodInitialized, nil); err != nil {
		return nil, fmt.Errorf("send initialized: %w", err)
	}
	return res, nil
}

func (c *Client) discover(ctx context.Context) (*mcp.InitializeResult, error) {
	c.mu.Lock()
	c.protocolVersion = mcp.ProtocolVersion20260728
	c.mu.Unlock()
	raw, err := c.Call(ctx, mcp.MethodDiscover, struct{}{})
	if err != nil {
		var rpc *mcp.Error
		var status *httpStatusError
		legacy := errors.As(err, &rpc) && rpc.Code == mcp.CodeMethodNotFound
		if errors.As(err, &rpc) && rpc.Code == -32022 {
			var versions struct{ Supported []string }
			legacy = json.Unmarshal(rpc.Data, &versions) == nil && slices.Contains(versions.Supported, mcp.ProtocolVersion20250618)
		}
		if !legacy && !(errors.As(err, &status) && (status.status == 400 || status.status == 404)) {
			return nil, err
		}
		c.mu.Lock()
		c.protocolVersion = ""
		c.mu.Unlock()
		return nil, nil
	}
	var d mcp.Discovery
	if json.Unmarshal(raw, &d) != nil || (d.ResultType != "" && d.ResultType != "complete") || !slices.Contains(d.SupportedVersions, mcp.ProtocolVersion20260728) {
		return nil, errors.New("invalid modern discovery result")
	}
	c.capabilities = d.Capabilities
	return &mcp.InitializeResult{ProtocolVersion: mcp.ProtocolVersion20260728, Capabilities: d.Capabilities, ServerInfo: d.Meta.Server, Instructions: d.Instructions}, nil
}

type httpStatusError struct {
	status int
	detail string
}

func (e *httpStatusError) Error() string { return e.detail }

// ListTools returns every tool, following pagination cursors.
func (c *Client) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	return listTools(ctx, c.Call)
}

// CallTool invokes a tool and returns the raw result object untouched.
func (c *Client) CallTool(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	return c.Call(ctx, mcp.MethodToolsCall, mcp.CallToolParams{Name: name, Arguments: args, Meta: relayFrom(ctx).Meta})
}

// Call sends a request and returns the raw JSON-RPC result.
// A JSON-RPC error from the upstream is returned as *mcp.Error.
func (c *Client) Call(ctx context.Context, method string, params any) (auditResult json.RawMessage, auditErr error) {
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
	resp, err := c.send(ctx, req)
	if err != nil {
		if ctx.Err() == nil {
			c.forgetPreference()
		}
		return nil, err
	}
	if !resp.IsResponse() || resp.ID.String() != id.String() {
		return nil, errors.New("upstream response does not match request id")
	}
	if resp.Error != nil {
		if resp.Error.Code == -32022 {
			c.forgetPreference()
		}
		return nil, resp.Error
	}
	if method != mcp.MethodDiscover && c.isModern() {
		var result struct{ ResultType string }
		if err := json.Unmarshal(resp.Result, &result); err != nil || (result.ResultType != "" && result.ResultType != "complete") {
			return nil, errors.New("unsupported upstream continuation")
		}
	}
	return resp.Result, nil
}

// Notify sends a notification (no response expected).
func (c *Client) Notify(ctx context.Context, method string, params any) error {
	n, err := mcp.NewRequest(mcp.ID{}, method, params)
	if err != nil {
		return err
	}
	_, err = c.send(ctx, n)
	return err
}

// Close terminates the upstream session (best effort).
func (c *Client) Close(ctx context.Context) error {
	sid := c.SessionID()
	if sid == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.url, nil)
	if err != nil {
		return err
	}
	c.applyHeaders(req, "")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxSSEEventBytes))
	c.mu.Lock()
	c.sessionID = ""
	c.mu.Unlock()
	return nil
}

func (c *Client) applyHeaders(req *http.Request, method string) {
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	c.mu.RLock()
	sid, ver := c.sessionID, c.protocolVersion
	c.mu.RUnlock()
	if sid != "" {
		req.Header.Set("Mcp-Session-Id", sid)
	}
	if ver != "" && method != mcp.MethodInitialize {
		req.Header.Set("MCP-Protocol-Version", ver)
	}
}

func (c *Client) isModern() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.protocolVersion == mcp.ProtocolVersion20260728
}

func (c *Client) send(ctx context.Context, msg *mcp.Message) (*mcp.Message, error) {
	modern := c.isModern()
	var target string
	if modern {
		if !msg.IsRequest() {
			return nil, errors.New("modern upstream requires a request")
		}
		params := map[string]json.RawMessage{}
		if len(msg.Params) > 0 {
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				return nil, err
			}
		}
		if params == nil {
			return nil, errors.New("upstream request parameters must be an object")
		}
		meta := map[string]json.RawMessage{}
		if raw := params["_meta"]; len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &meta); err != nil {
				return nil, err
			}
		}
		meta["io.modelcontextprotocol/protocolVersion"] = json.RawMessage(`"2026-07-28"`)
		meta["io.modelcontextprotocol/clientInfo"] = json.RawMessage(`{"name":"vmcp-gateway","version":"0.1.0"}`)
		meta["io.modelcontextprotocol/clientCapabilities"] = json.RawMessage(`{}`)
		params["_meta"], _ = json.Marshal(meta)
		msg.Params, _ = json.Marshal(params)
		switch msg.Method {
		case "tools/call", "prompts/get":
			_ = json.Unmarshal(params["name"], &target)
		case "resources/read":
			_ = json.Unmarshal(params["uri"], &target)
		}
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	c.applyHeaders(req, msg.Method)
	if msg.Method == mcp.MethodToolsCall {
		callercontext.ApplyHeaders(ctx, req)
	}

	if modern {
		req.Header.Del("Mcp-Session-Id")
		req.Header.Del("Mcp-Name")
		req.Header.Set("Mcp-Method", msg.Method)
		req.Header.Set("MCP-Protocol-Version", mcp.ProtocolVersion20260728)
		if target != "" {
			req.Header.Set("Mcp-Name", mcp.RoutingValue(target))
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post %s: %w", msg.Method, err)
	}
	defer resp.Body.Close()

	if msg.Method == mcp.MethodInitialize {
		if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
			c.mu.Lock()
			c.sessionID = sid
			c.mu.Unlock()
		}
	}

	if resp.StatusCode == http.StatusNotFound && c.SessionID() != "" {
		return nil, ErrSessionExpired
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var snippet []byte
		if modern && (resp.StatusCode == 400 || resp.StatusCode == 404) {
			snippet, err = readBounded(resp.Body)
			if err != nil {
				return nil, err
			}
			if reply, err := mcp.Decode(snippet); err == nil && reply.Error != nil && reply.ID.String() == msg.ID.String() {
				return reply, nil
			}
			if len(snippet) > 512 {
				snippet = snippet[:512]
			}
		} else {
			snippet, _ = io.ReadAll(io.LimitReader(resp.Body, 512))
		}
		return nil, &httpStatusError{resp.StatusCode, fmt.Sprintf("upstream %s returned %d: %s", msg.Method, resp.StatusCode, strings.TrimSpace(string(snippet)))}
	}

	if msg.IsNotification() || msg.IsResponse() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxSSEEventBytes))
		return nil, nil
	}

	mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	switch mt {
	case "application/json":
		data, err := readBounded(resp.Body)
		if err != nil {
			return nil, err
		}
		out, err := mcp.Decode(data)
		if err != nil {
			return nil, err
		}
		return out, nil
	case "text/event-stream":
		return c.readSSEContext(ctx, resp.Body, msg.ID)
	default:
		return nil, fmt.Errorf("upstream returned unsupported content type %q", mt)
	}
}

// readSSE reads events until the response matching id arrives.
func (c *Client) readSSE(r io.Reader, id mcp.ID) (*mcp.Message, error) {
	return c.readSSEContext(context.Background(), r, id)
}
func (c *Client) readSSEContext(ctx context.Context, r io.Reader, id mcp.ID) (*mcp.Message, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), maxSSEEventBytes)

	var data []string
	frameBytes := 0
	messageCount := 0
	requestSlots := make(chan struct{}, 32)
	flush := func() (*mcp.Message, error) {
		if len(data) == 0 {
			return nil, nil
		}
		payload := strings.Join(data, "\n")
		data = data[:0]
		frameBytes = 0
		messageCount++
		if messageCount > 256 {
			return nil, errors.New("upstream stream message limit exceeded")
		}
		m, err := mcp.Decode([]byte(payload))
		if err != nil {
			return nil, err
		}
		if m.IsResponse() {
			if m.ID.String() != id.String() {
				return nil, errors.New("upstream response does not match request id")
			}
			return m, nil
		}
		if m.IsRequest() {
			select {
			case requestSlots <- struct{}{}:
			default:
				return nil, errors.New("upstream pending request limit exceeded")
			}
			go func() { defer func() { <-requestSlots }(); _ = handleMessage(ctx, m, c.Send, c.OnServerMessage) }()
			return nil, nil
		}
		if err := handleMessage(ctx, m, c.Send, c.OnServerMessage); err != nil {
			return nil, err
		}
		return nil, nil
	}

	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if m, err := flush(); m != nil || err != nil {
				return m, err
			}
		case strings.HasPrefix(line, "data:"):
			frameBytes += len(line)
			if frameBytes > maxSSEEventBytes {
				return nil, errors.New("upstream SSE frame exceeds limit")
			}
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if m, err := flush(); m != nil || err != nil {
		return m, err
	}
	return nil, io.ErrUnexpectedEOF
}
