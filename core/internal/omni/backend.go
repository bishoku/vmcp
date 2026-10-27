package omni

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Backend is a private interface within the same logical gateway resource.
// End-user authorization is always rechecked by the data plane.
type Backend struct {
	URL, Secret string
	Client      *http.Client
}

func NewBackend(raw, secret string) (*Backend, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(secret) < 32 {
		return nil, errors.New("valid data-plane URL and 32-character Omni secret required")
	}
	return &Backend{URL: strings.TrimRight(raw, "/"), Secret: secret, Client: &http.Client{Timeout: 70 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

type HTTPError struct{ Status int }

func (e *HTTPError) Error() string { return fmt.Sprintf("gateway request failed (%d)", e.Status) }
func (b *Backend) Request(ctx context.Context, method, path, token, ip, trace string, body any, out any) error {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	r, err := http.NewRequestWithContext(ctx, method, b.URL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	r.Header.Set("X-VMCP-Omni-Secret", b.Secret)
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", token)
	}
	if ip != "" {
		r.Header.Set("X-VMCP-Client-IP", ip)
	}
	if trace != "" {
		r.Header.Set("traceparent", trace)
	}
	res, err := b.Client.Do(r)
	if err != nil {
		return errors.New("gateway unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return &HTTPError{res.StatusCode}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(res.Body, 64<<20)).Decode(out)
}
