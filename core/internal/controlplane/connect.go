package controlplane

import (
	"context"
	"net/http"
	"time"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/oauthtoken"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/tlsconf"
	"github.com/bishoku/vmcp/core/internal/upstream"
)

const probeTimeout = 20 * time.Second

type probeResult struct {
	Tools           []mcp.Tool
	ServerName      string
	ServerVersion   string
	ProtocolVersion string
	Latency         time.Duration
}

// probe connects to an MCP server, performs the handshake and lists its tools.
// probeAny tries the URLs of an upstream in order and returns the first that answers.
func probeAny(ctx context.Context, urls []string, transport string, headers map[string]string, tl tlsconf.Settings, oc *oauthtoken.Config) (*probeResult, error) {
	var res *probeResult
	var err error
	for _, u := range urls {
		if res, err = probe(ctx, u, transport, headers, tl, oc); err == nil {
			return res, nil
		}
	}
	return nil, err
}

func probe(ctx context.Context, url, transport string, headers map[string]string, tl tlsconf.Settings, oc *oauthtoken.Config) (*probeResult, error) {
	c, err := upstream.OpenAuth(transport, url, headers, tl, oc)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	start := time.Now()
	defer func() {
		closeCtx, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		_ = c.Close(closeCtx)
	}()

	init, err := c.Initialize(ctx)
	if err != nil {
		return nil, err
	}
	tools, err := c.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	return &probeResult{
		Tools:           tools,
		ServerName:      init.ServerInfo.Name,
		ServerVersion:   init.ServerInfo.Version,
		ProtocolVersion: init.ProtocolVersion,
		Latency:         time.Since(start),
	}, nil
}

func (s *Server) refreshTools(w http.ResponseWriter, r *http.Request) error {
	slug := r.PathValue("slug")
	conn, err := s.Store.Reader().UpstreamConnInfo(r.Context(), slug)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	res, err := probeAny(r.Context(), append([]string{conn.URL}, conn.FallbackURLs...), conn.Transport, conn.Headers, conn.TLS, conn.OAuth)
	if err != nil {
		msg := err.Error()
		_ = s.Store.Do(r.Context(), func(tx *store.Tx) error { return tx.SetRefreshStatus(r.Context(), slug, now, &msg) })
		return apierror.New(http.StatusBadGateway, apierror.CodeUpstreamUnreach, "Upstream unreachable", msg)
	}

	var result *store.RefreshResult
	if err := s.Store.Do(r.Context(), func(tx *store.Tx) error {
		var err error
		if result, err = tx.ReplaceTools(r.Context(), slug, res.Tools, now); err != nil {
			return err
		}
		return tx.SetRefreshStatus(r.Context(), slug, now, nil)
	}); err != nil {
		return err
	}
	s.setRevision(w, r)
	return writeJSON(w, http.StatusOK, result)
}

type connectionTestWire struct {
	URL       *string           `json:"url"`
	Transport *string           `json:"transport"`
	Headers   map[string]string `json:"headers"`
	TLS       *tlsWire          `json:"tls"`
	Auth      *authWire         `json:"auth"`
}

func (s *Server) connectionTest(w http.ResponseWriter, r *http.Request) error {
	var body connectionTestWire
	if err := decode(r, &body); err != nil {
		return err
	}
	if body.URL == nil {
		return apierror.Validation([]apierror.FieldError{required("/url")})
	}
	in := store.UpstreamInput{Name: "connection test", URL: *body.URL, Transport: "streamable_http", Headers: body.Headers}
	if body.Transport != nil {
		in.Transport = *body.Transport
	}
	if errs := store.ValidateUpstreamInput(in); len(errs) > 0 {
		return apierror.Validation(errs)
	}

	in.TLS = body.TLS.settings()
	tl, err := store.ResolveTLS(in.TLS, tlsconf.Settings{})
	if err != nil {
		return err
	}
	oc, err := store.ResolveOAuth(body.Auth.config(), oauthtoken.Config{}, in.Headers)
	if err != nil {
		return err
	}
	var ocp *oauthtoken.Config
	if !oc.IsZero() {
		ocp = &oc
	}
	res, err := probe(r.Context(), in.URL, in.Transport, in.Headers, tl, ocp)
	if err != nil {
		return writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
	}
	tools := make([]mcp.Tool, len(res.Tools))
	copy(tools, res.Tools)
	return writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"serverInfo":      map[string]string{"name": res.ServerName, "version": res.ServerVersion},
		"protocolVersion": res.ProtocolVersion,
		"latencyMs":       res.Latency.Milliseconds(),
		"tools":           tools,
	})
}
