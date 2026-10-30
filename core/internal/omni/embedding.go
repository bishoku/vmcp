package omni

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPEmbedder supports an explicitly configured OpenAI-compatible embeddings API,
// including an institution-hosted model. No provider or model is selected implicitly.
type HTTPEmbedder struct {
	Endpoint, Model, Key string
	Client               *http.Client
	Profile              EmbeddingProfile
}

func NewHTTPEmbedder(endpoint, model, key string, insecureSkipVerify bool) (*HTTPEmbedder, error) {
	u, e := url.Parse(endpoint)
	if e != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || model == "" {
		return nil, errors.New("valid embedding endpoint and model required")
	}
	// Use a private transport: this setting must not affect data-plane or other HTTP clients.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: insecureSkipVerify} // Explicit operator opt-in for private embedding endpoints.
	return &HTTPEmbedder{Endpoint: endpoint, Model: model, Key: key, Client: &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (e *HTTPEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return e.embed(ctx, texts, e.Profile.DocumentPrefix)
}
func (e *HTTPEmbedder) embed(ctx context.Context, texts []string, prefix string) ([][]float32, error) {
	input := make([]string, len(texts))
	for i, s := range texts {
		r := []rune(s)
		if len(r) > 12000 {
			r = r[:12000]
		}
		input[i] = prefix + string(r)
	}
	b, _ := json.Marshal(map[string]any{"model": e.Model, "input": input})
	req, err := http.NewRequestWithContext(ctx, "POST", e.Endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, e.failure("invalid_request")
	}
	req.Header.Set("Content-Type", "application/json")
	if e.Key != "" {
		req.Header.Set("Authorization", "Bearer "+e.Key)
	}
	traceProvider(ctx, "embedding", "request", b, e.Key, 0)
	res, err := e.Client.Do(req)
	if err != nil {
		return nil, e.failure(transportReason(err))
	}
	tapProvider(ctx, "embedding", e.Key, res)
	defer res.Body.Close()
	if res.StatusCode != 200 {
		attrs := []slog.Attr{slog.Int("http_status", res.StatusCode), slog.String("http_status_text", http.StatusText(res.StatusCode))}
		// Provider messages and raw bodies can echo input or credentials. Only retain
		// short machine-readable error identifiers from an OpenAI-compatible response.
		var body struct {
			Error struct {
				Code any    `json:"code"`
				Type string `json:"type"`
			} `json:"error"`
		}
		if json.NewDecoder(io.LimitReader(res.Body, 16<<10)).Decode(&body) == nil {
			if code, ok := body.Error.Code.(string); ok && safeErrorToken(code, e.Key, texts) {
				attrs = append(attrs, slog.String("provider_code", code))
			}
			if safeErrorToken(body.Error.Type, e.Key, texts) {
				attrs = append(attrs, slog.String("provider_type", body.Error.Type))
			}
		}
		return nil, e.failure("http_status", attrs...)
	}
	var payload struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&payload); err != nil {
		reason := "invalid_json"
		attrs := []slog.Attr{slog.Int("http_status", res.StatusCode)}
		var syntax *json.SyntaxError
		var fieldType *json.UnmarshalTypeError
		switch {
		case errors.Is(err, io.EOF):
			reason = "empty_response"
		case errors.Is(err, io.ErrUnexpectedEOF):
			reason = "truncated_response"
		case errors.As(err, &syntax):
			attrs = append(attrs, slog.Int64("json_offset", syntax.Offset))
		case errors.As(err, &fieldType):
			reason = "invalid_response_type"
			attrs = append(attrs, slog.Int64("json_offset", fieldType.Offset))
		default:
			reason = transportReason(err)
		}
		return nil, e.failure(reason, attrs...)
	}
	if len(payload.Data) != len(texts) {
		return nil, e.failure("embedding_count_mismatch", slog.Int("expected_count", len(texts)), slog.Int("actual_count", len(payload.Data)))
	}
	out := make([][]float32, len(texts))
	dimension := 0
	for _, d := range payload.Data {
		if d.Index < 0 || d.Index >= len(out) || out[d.Index] != nil || len(d.Embedding) == 0 || len(d.Embedding) > 4096 {
			return nil, e.failure("invalid_embedding_dimensions", slog.Int("response_index", d.Index), slog.Int("dimensions", len(d.Embedding)))
		}
		if dimension == 0 {
			dimension = len(d.Embedding)
		}
		if dimension != len(d.Embedding) {
			return nil, e.failure("inconsistent_embedding_dimensions", slog.Int("expected_dimensions", dimension), slog.Int("actual_dimensions", len(d.Embedding)))
		}
		for _, v := range d.Embedding {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return nil, e.failure("nonfinite_embedding")
			}
		}
		out[d.Index] = d.Embedding
	}
	if _, err := validateVectors(out, len(texts), e.Profile.Dimensions); err != nil {
		diagnostic := err.(*embeddingError)
		return nil, e.failure(diagnostic.reason, diagnostic.attrs...)
	}
	return out, nil
}

func safeErrorToken(value, key string, texts []string) bool {
	if len(value) == 0 || len(value) > 80 || (key != "" && strings.Contains(value, key)) {
		return false
	}
	for _, text := range texts {
		if text != "" && strings.Contains(value, text) {
			return false
		}
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.') {
			return false
		}
	}
	return true
}
