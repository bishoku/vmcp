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
	"strings"
	"sync"
	"sync/atomic"

	"github.com/bishoku/vmcp/core/internal/mcp"
)

// ErrSessionExpired means the upstream no longer knows our session (HTTP 404).
// Callers should re-initialize and retry.
var ErrSessionExpired = errors.New("upstream session expired")

const maxSSEEventBytes = 10 << 20

// Client is a Streamable HTTP MCP client bound to one upstream and one session.
type Client struct {
	url     string
	headers map[string]string
	http    *http.Client
	nextID  atomic.Int64

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
	return &Client{url: url, headers: headers, http: hc}
}

// SessionID returns the current upstream session id, if any.
func (c *Client) SessionID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionID
}

// Initialize performs the MCP handshake and records the session.
func (c *Client) Initialize(ctx context.Context) (*mcp.InitializeResult, error) {
	params := mcp.InitializeParams{
		ProtocolVersion: mcp.SupportedVersions[0],
		Capabilities:    json.RawMessage(`{}`),
		ClientInfo:      mcp.Implementation{Name: "vmcp-gateway", Version: "0.1.0"},
	}
	raw, err := c.Call(ctx, mcp.MethodInitialize, params)
	if err != nil {
		return nil, err
	}
	var res mcp.InitializeResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("decode initialize result: %w", err)
	}
	c.mu.Lock()
	c.protocolVersion = res.ProtocolVersion
	c.mu.Unlock()

	if err := c.Notify(ctx, mcp.MethodInitialized, nil); err != nil {
		return nil, fmt.Errorf("send initialized: %w", err)
	}
	return &res, nil
}

// ListTools returns every tool, following pagination cursors.
func (c *Client) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	var all []mcp.Tool
	cursor := ""
	for {
		raw, err := c.Call(ctx, mcp.MethodToolsList, mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		var page mcp.ListToolsResult
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("decode tools/list: %w", err)
		}
		all = append(all, page.Tools...)
		if page.NextCursor == "" {
			return all, nil
		}
		cursor = page.NextCursor
	}
}

// CallTool invokes a tool and returns the raw result object untouched.
func (c *Client) CallTool(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	return c.Call(ctx, mcp.MethodToolsCall, mcp.CallToolParams{Name: name, Arguments: args})
}

// Call sends a request and returns the raw JSON-RPC result.
// A JSON-RPC error from the upstream is returned as *mcp.Error.
func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := mcp.NewIntID(c.nextID.Add(1))
	req, err := mcp.NewRequest(id, method, params)
	if err != nil {
		return nil, err
	}
	resp, err := c.send(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, resp.Error
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
	_, _ = io.Copy(io.Discard, resp.Body)
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

func (c *Client) send(ctx context.Context, msg *mcp.Message) (*mcp.Message, error) {
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
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("upstream %s returned %d: %s", msg.Method, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	if msg.IsNotification() {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, nil
	}

	mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	switch mt {
	case "application/json":
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		out, err := mcp.Decode(data)
		if err != nil {
			return nil, err
		}
		return out, nil
	case "text/event-stream":
		return c.readSSE(resp.Body, msg.ID)
	default:
		return nil, fmt.Errorf("upstream returned unsupported content type %q", mt)
	}
}

// readSSE reads events until the response matching id arrives.
func (c *Client) readSSE(r io.Reader, id mcp.ID) (*mcp.Message, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), maxSSEEventBytes)

	var data []string
	flush := func() (*mcp.Message, error) {
		if len(data) == 0 {
			return nil, nil
		}
		payload := strings.Join(data, "\n")
		data = data[:0]
		m, err := mcp.Decode([]byte(payload))
		if err != nil {
			return nil, err
		}
		if m.IsResponse() && m.ID.String() == id.String() {
			return m, nil
		}
		if c.OnServerMessage != nil {
			c.OnServerMessage(m)
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
