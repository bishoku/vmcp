package virtual

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/bishoku/vmcp/core/internal/netpolicy"
	"io"
	"net/http"
	"net/url"
	"time"
)

// PolicyDecision contains metadata and a payload digest; raw content never leaves
// the gateway through this hook. The external service owns approval workflows.
type PolicyDecision struct {
	Actor    string `json:"actor"`
	Server   string `json:"server"`
	Method   string `json:"method"`
	Object   string `json:"object"`
	Revision int64  `json:"revision"`
	Digest   string `json:"sha256"`
}
type ApprovalPolicy struct {
	endpoint string
	client   *http.Client
}

func NewApprovalPolicy(endpoint string) (*ApprovalPolicy, error) {
	if endpoint == "" {
		return nil, nil
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return nil, errors.New("approval URL must use HTTPS without credentials or fragment")
	}
	return &ApprovalPolicy{endpoint: endpoint, client: netpolicy.Client(&http.Client{Timeout: 2 * time.Second})}, nil
}
func (p *ApprovalPolicy) Check(ctx context.Context, d PolicyDecision, content []byte) error {
	if p == nil {
		return nil
	}
	hash := sha256.Sum256(content)
	d.Digest = hex.EncodeToString(hash[:])
	raw, _ := json.Marshal(d)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewReader(raw))
	if err != nil {
		return errors.New("approval unavailable")
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		return errors.New("approval unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("approval denied")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	if err != nil || len(body) > 4096 {
		return errors.New("invalid approval response")
	}
	var result struct {
		Allow bool `json:"allow"`
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if dec.Decode(&result) != nil || !result.Allow {
		return errors.New("approval denied")
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return errors.New("invalid approval response")
	}
	return nil
}
