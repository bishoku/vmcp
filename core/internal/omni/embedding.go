package omni

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"time"
)

// HTTPEmbedder supports an explicitly configured OpenAI-compatible embeddings API,
// including an institution-hosted model. No provider or model is selected implicitly.
type HTTPEmbedder struct {
	Endpoint, Model, Key string
	Client               *http.Client
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
	input := make([]string, len(texts))
	for i, s := range texts {
		r := []rune(s)
		if len(r) > 12000 {
			r = r[:12000]
		}
		input[i] = string(r)
	}
	b, _ := json.Marshal(map[string]any{"model": e.Model, "input": input})
	req, err := http.NewRequestWithContext(ctx, "POST", e.Endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.Key != "" {
		req.Header.Set("Authorization", "Bearer "+e.Key)
	}
	res, err := e.Client.Do(req)
	if err != nil {
		return nil, errors.New("embedding service unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, errors.New("embedding service rejected request")
	}
	var payload struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&payload) != nil || len(payload.Data) != len(texts) {
		return nil, errors.New("invalid embedding response")
	}
	out := make([][]float32, len(texts))
	dimension := 0
	for _, d := range payload.Data {
		if d.Index < 0 || d.Index >= len(out) || out[d.Index] != nil || len(d.Embedding) == 0 || len(d.Embedding) > 4096 {
			return nil, errors.New("invalid embedding dimensions")
		}
		if dimension == 0 {
			dimension = len(d.Embedding)
		}
		if dimension != len(d.Embedding) {
			return nil, errors.New("inconsistent embedding dimensions")
		}
		for _, v := range d.Embedding {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return nil, errors.New("nonfinite embedding")
			}
		}
		out[d.Index] = d.Embedding
	}
	return out, nil
}
