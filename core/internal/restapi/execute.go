package restapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/netpolicy"
	"github.com/bishoku/vmcp/core/internal/oauthtoken"
	"github.com/bishoku/vmcp/core/internal/tlsconf"
)

const MaxResponseBytes = 1 << 20

type Identity struct{ Type, ID, Issuer, Subject, Tenant, VirtualServer string }

func (i Identity) AgentID() string {
	switch i.Type {
	case "agent-key":
		return i.ID
	case "oauth":
		if i.Issuer != "" && i.Subject != "" {
			raw, _ := json.Marshal([]string{i.Tenant, i.Issuer, i.Subject})
			sum := sha256.Sum256(raw)
			return "oauth:" + hex.EncodeToString(sum[:])
		}
	}
	return ""
}

type Preview struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body,omitempty"`
}

// Build performs schema validation and URL encoding without network access or authentication side effects.
func (c *Compiled) Build(base, name string, args json.RawMessage, common map[string]string, identity Identity) (*http.Request, error) {
	op, ok := c.Operations[name]
	if !ok {
		return nil, errors.New("REST operation unavailable")
	}
	var values map[string]any
	if decodeJSON(args, &values) != nil || values == nil {
		return nil, errors.New("arguments must be a JSON object")
	}
	if err := op.Input.Validate(values); err != nil {
		return nil, errors.New("arguments do not match the operation input schema")
	}
	if c.Config.RequireIdentity && identity.AgentID() == "" {
		return nil, errors.New("verified caller identity is required")
	}
	path := op.Definition.Path
	q := url.Values{}
	for _, p := range op.Definition.Parameters {
		v, ok := values[p.Name]
		if !ok {
			continue
		}
		value := fmt.Sprint(v)
		if p.In == "path" {
			if value == "." || value == ".." || strings.ContainsAny(value, "/\\%\x00") {
				return nil, errors.New("unsafe path parameter")
			}
			path = strings.ReplaceAll(path, "{"+p.Name+"}", url.PathEscape(value))
		} else {
			q.Set(p.Name, value)
		}
	}
	target := strings.TrimSuffix(base, "/") + path
	if query := q.Encode(); query != "" {
		target += "?" + query
	}
	var body []byte
	if b, ok := values["body"]; ok {
		body, _ = json.Marshal(b)
	}
	req, err := http.NewRequest(op.Definition.Method, target, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("cannot construct REST request")
	}
	// A non-rewindable body prevents net/http from replaying GET or requests
	// with Idempotency-Key after losing a response on a reused connection.
	// Keep this wrapper for empty bodies too; retain connection pooling.
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.GetBody = nil
	req.Header = Headers(common, op.Definition.Headers)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Config.ForwardIdentity && identity.AgentID() != "" {
		req.Header.Set("X-VMCP-Agent-Id", identity.AgentID())
		req.Header.Set("X-VMCP-Auth-Type", identity.Type)
		req.Header.Set("X-VMCP-Virtual-Server", identity.VirtualServer)
	}
	return req, nil
}
func (c *Compiled) Preview(base, name string, args json.RawMessage, common map[string]string) (Preview, error) {
	// Operator preview never impersonates an agent. Missing required identity is represented separately by the UI.
	clone := *c
	clone.Config.RequireIdentity = false
	req, err := clone.Build(base, name, args, common, Identity{})
	if err != nil {
		return Preview{}, err
	}
	result := Preview{Method: req.Method, URL: req.URL.String(), Headers: map[string]string{}}
	for k := range req.Header {
		result.Headers[k] = "[configured]"
	}
	if c.Config.Auth.Type != "none" {
		result.Headers["Authorization"] = "[managed secret]"
	}
	if c.Config.ForwardIdentity {
		result.Headers["X-VMCP-Agent-Id"] = "[verified caller, when available]"
	}
	if req.Body != nil {
		result.Body, _ = io.ReadAll(req.Body)
	}
	return result, nil
}
func failure(message string) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{"isError": true, "content": []any{map[string]string{"type": "text", "text": message}}})
	return raw
}
func (c *Compiled) Execute(ctx context.Context, base, name string, args json.RawMessage, headers map[string]string, tl tlsconf.Settings, identity Identity, authorize func() error) (auditResult json.RawMessage, auditErr error) {
	req, err := c.Build(base, name, args, headers, identity)
	if err != nil {
		return failure(err.Error()), nil
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.Config.TimeoutSeconds)*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	observing := audit.ObservationEnabled(ctx)
	var requestBody json.RawMessage
	if req.Body != nil && observing {
		raw, _ := io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(raw))
		if len(raw) > 0 {
			requestBody = raw
		}
	}
	complete, startErr := audit.Observe(ctx, req.Method, map[string]any{"operation": name, "path": req.URL.Path, "query": req.URL.Query(), "body": requestBody})
	if startErr != nil {
		return nil, startErr
	}
	var observedResponse json.RawMessage
	var observedStatus int
	defer func() {
		if !observing {
			return
		}
		content := auditResult
		if observedStatus != 0 {
			var body any = string(observedResponse)
			if json.Valid(observedResponse) {
				body = observedResponse
			}
			content, _ = json.Marshal(map[string]any{"statusCode": observedStatus, "body": body})
		}
		if e := complete(content, auditErr); e != nil {
			auditResult = nil
			auditErr = e
		}
	}()

	var tokenSource *oauthtoken.Source
	var token string
	switch a := c.Config.Auth; a.Type {
	case "basic":
		req.SetBasicAuth(a.Username, a.Password)
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+a.Token)
	case "oauth2":
		tokenSource = oauthtoken.For(*a.OAuth())
		token, err = tokenSource.Token(ctx)
		if err != nil {
			return failure("REST authentication service unavailable"), nil
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	hc, err := tl.Client()
	if err != nil {
		return nil, &ExecutionError{Stage: "tls_configuration", cause: err}
	}
	client := netpolicy.Client(hc)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if authorize != nil {
		if err := authorize(); err != nil {
			return nil, err
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, &ExecutionError{Stage: "request", cause: err}
	}
	defer resp.Body.Close()
	observedStatus = resp.StatusCode
	if resp.StatusCode == 401 && tokenSource != nil {
		tokenSource.Invalidate(token)
	} // Never replay an operation, including after token renewal.
	op := c.Operations[name].Definition
	if resp.StatusCode < 200 || resp.StatusCode > 299 || (len(op.SuccessStatuses) > 0 && !slices.Contains(op.SuccessStatuses, resp.StatusCode)) {
		if observing {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
			observedResponse = body
		}
		return failure(fmt.Sprintf("REST service returned HTTP %d", resp.StatusCode)), nil
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil || len(raw) > MaxResponseBytes {
		return failure("REST response exceeded the size limit or could not be read; operation may have completed"), nil
	}
	observedResponse = raw
	var value any
	if resp.StatusCode != 204 {
		if decodeJSON(raw, &value) != nil {
			return failure("REST service returned invalid JSON"), nil
		}
	}
	if err := c.Operations[name].Output.Validate(value); err != nil {
		return failure("REST response does not match the configured schema; operation may have completed"), nil
	}
	if len(op.ResponseFields) > 0 {
		obj, ok := value.(map[string]any)
		if !ok {
			return failure("REST response must be an object"), nil
		}
		selected := map[string]any{}
		for _, field := range op.ResponseFields {
			if v, ok := obj[field]; ok {
				selected[field] = v
			}
		}
		value = selected
	}
	structured := map[string]any{"result": value}
	text, _ := json.Marshal(structured)
	return json.Marshal(map[string]any{"structuredContent": structured, "content": []any{map[string]string{"type": "text", "text": string(text)}}})
}

// Decode one JSON value without rounding large integer IDs.
func decodeJSON(raw []byte, into any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(into); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("expected one JSON value")
	}
	return nil
}
