package invocation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/bishoku/vmcp/core/internal/netpolicy"
)

type Requirements struct {
	StepUp   *StepUp `json:"stepUp,omitempty"`
	Approval bool    `json:"approval"`
}
type Request struct {
	IdentitySource        string                     `json:"identitySource"`
	User                  *Identity                  `json:"user,omitempty"`
	CallerContext         map[string]string          `json:"callerContext,omitempty"`
	Requirements          Requirements               `json:"requirements"`
	Version               string                     `json:"version"`
	Phase                 string                     `json:"phase"` // check creates/looks up consent; consume atomically spends it
	OperationID           string                     `json:"operationId"`
	Identity              Identity                   `json:"application"`
	Tenant                string                     `json:"tenant"`
	Upstream              string                     `json:"upstreamId"`
	Tool                  string                     `json:"tool"`
	Method                string                     `json:"method"`
	Server                string                     `json:"server"`
	ConfigurationRevision int64                      `json:"configurationRevision"`
	PolicyRevision        int64                      `json:"policyRevision"`
	Digest                string                     `json:"sha256"`
	Details               map[string]json.RawMessage `json:"details"`
}
type Decision struct {
	VerifiedUser *Identity `json:"verifiedUser,omitempty"`
	Status       string    `json:"status"`
	ApprovalID   string    `json:"approvalId"`
	ExpiresAt    time.Time `json:"expiresAt"`
	URL          string    `json:"url,omitempty"`
}

// Content hashes normalized JSON with sorted keys and exact number lexemes. It
// never converts monetary values through float64. Equivalent numeric spellings
// can require a new approval, which is deliberately conservative.
func Content(raw []byte, fields []string) (string, map[string]json.RawMessage, error) {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	value, err := decodeUnique(dec, 0)
	if err != nil {
		return "", nil, err
	}
	if dec.Decode(new(any)) != io.EOF {
		return "", nil, errors.New("invalid JSON content")
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", nil, err
	}
	hash := sha256.Sum256(canonical)
	var args map[string]json.RawMessage
	if err = json.Unmarshal(canonical, &args); err != nil {
		return "", nil, errors.New("arguments must be an object")
	}
	details := map[string]json.RawMessage{}
	for _, key := range fields {
		if v, ok := args[key]; ok {
			details[key] = v
		}
	}
	return hex.EncodeToString(hash[:]), details, nil
}
func Call(ctx context.Context, svc Service, req Request) (Decision, error) {
	var out Decision
	if svc.Token == "" {
		return out, errors.New("approval service credentials unavailable")
	}
	client, err := approvalClient(svc.CAPEM)
	if err != nil {
		return out, err
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, svc.URL, bytes.NewReader(raw))
	if err != nil {
		return out, err
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+svc.Token)
	res, err := client.Do(r)
	if err != nil {
		return out, errors.New("approval service unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return out, errors.New("approval service rejected request")
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 8193))
	if err != nil || len(body) > 8192 {
		return out, errors.New("invalid approval response")
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&out) != nil || d.Decode(new(any)) != io.EOF {
		return out, errors.New("invalid approval response")
	}
	if out.Status != "allowed" && out.Status != "denied" && out.Status != "pending" && out.Status != "step_up_required" {
		return out, errors.New("invalid approval status")
	}
	if out.Status != "denied" && (out.ApprovalID == "" || !out.ExpiresAt.After(time.Now())) {
		return out, errors.New("expired or missing approval")
	}
	if out.URL != "" {
		u, e := url.Parse(out.URL)
		origin, e2 := url.Parse(svc.ApprovalOrigin)
		if e != nil || e2 != nil || u.Scheme != "https" || u.User != nil || u.Host != origin.Host || u.Fragment != "" {
			return out, errors.New("untrusted approval URL")
		}
	}
	return out, nil
}

var clientsMu sync.Mutex
var approvalClients = map[string]*http.Client{}

func approvalClient(ca string) (*http.Client, error) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	if c := approvalClients[ca]; c != nil {
		return c, nil
	}
	if len(approvalClients) >= 256 {
		return nil, errors.New("approval client capacity exceeded")
	}
	svc := Service{CAPEM: ca}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if svc.CAPEM != "" {
		roots, err := x509.SystemCertPool()
		if err != nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM([]byte(svc.CAPEM)) {
			return nil, errors.New("invalid approval CA")
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	}
	client := netpolicy.Client(&http.Client{Transport: transport, Timeout: 2 * time.Second})
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	approvalClients[ca] = client
	return client, nil
}

// Reject duplicate keys rather than approving an interpretation which an
// upstream parser could interpret differently.
func decodeUnique(d *json.Decoder, depth int) (any, error) {
	if depth > 64 {
		return nil, errors.New("JSON nesting exceeds limit")
	}
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	delimiter, container := token.(json.Delim)
	if !container {
		return token, nil
	}
	switch delimiter {
	case '{':
		object := map[string]any{}
		for d.More() {
			key, e := d.Token()
			if e != nil {
				return nil, e
			}
			name, ok := key.(string)
			if !ok {
				return nil, errors.New("invalid object key")
			}
			if _, exists := object[name]; exists {
				return nil, errors.New("duplicate object key")
			}
			v, e := decodeUnique(d, depth+1)
			if e != nil {
				return nil, e
			}
			object[name] = v
		}
		end, e := d.Token()
		if e != nil || end != json.Delim('}') {
			return nil, errors.New("invalid object")
		}
		return object, nil
	case '[':
		array := []any{}
		for d.More() {
			v, e := decodeUnique(d, depth+1)
			if e != nil {
				return nil, e
			}
			array = append(array, v)
		}
		end, e := d.Token()
		if e != nil || end != json.Delim(']') {
			return nil, errors.New("invalid array")
		}
		return array, nil
	}
	return nil, errors.New("invalid JSON delimiter")
}
