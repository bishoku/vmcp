package mockupstream

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

	"github.com/bishoku/vmcp/core/internal/netpolicy"
)

type Client struct {
	Base string
	HTTP *http.Client
}
type RemoteMapping struct {
	ID       string                     `json:"id"`
	Metadata map[string]json.RawMessage `json:"metadata"`
}

func (m RemoteMapping) Managed() Metadata {
	var out Metadata
	_ = json.Unmarshal(m.Metadata["vmcp"], &out)
	return out
}
func NewClient(base string) *Client {
	return &Client{Base: strings.TrimSuffix(base, "/"), HTTP: netpolicy.Client(&http.Client{Timeout: 10 * time.Second})}
}
func (c *Client) request(ctx context.Context, method, path string, body any, out any) error {
	if err := ValidateEndpoint(c.Base); err != nil {
		return err
	}
	var raw []byte
	var err error
	if body != nil {
		raw, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return errors.New("WireMock is unreachable or timed out")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		data, _ := io.ReadAll(io.LimitReader(res.Body, 16384))
		var problem struct {
			Errors []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
				Source struct {
					Pointer string `json:"pointer"`
				} `json:"source"`
			} `json:"errors"`
		}
		_ = json.Unmarshal(data, &problem)
		if len(problem.Errors) > 0 {
			detail := problem.Errors[0].Title + " " + problem.Errors[0].Source.Pointer + ": " + problem.Errors[0].Detail
			if len(detail) > 400 {
				detail = detail[:400]
			}
			return fmt.Errorf("WireMock admin request failed (HTTP %d): %s", res.StatusCode, detail)
		}
		return fmt.Errorf("WireMock admin request failed (HTTP %d)", res.StatusCode)
	}
	if out != nil {
		data, err := io.ReadAll(io.LimitReader(res.Body, (4<<20)+1))
		if err != nil || len(data) > 4<<20 {
			return errors.New("WireMock admin response exceeds limits")
		}
		if json.Unmarshal(data, out) != nil {
			return errors.New("invalid WireMock admin response")
		}
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
	}
	return nil
}
func (c *Client) List(ctx context.Context) ([]RemoteMapping, error) {
	all := []RemoteMapping{}
	for offset := 0; offset < 10000; offset += 200 {
		var page struct {
			Mappings []RemoteMapping `json:"mappings"`
		}
		if err := c.request(ctx, "GET", fmt.Sprintf("/__admin/mappings?limit=200&offset=%d", offset), nil, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Mappings...)
		if len(page.Mappings) < 200 {
			return all, nil
		}
	}
	return nil, errors.New("WireMock mapping inventory exceeds 10000 mappings")
}

// Reconcile never uses global reset/import replacement endpoints. A rejected
// rule does not block publication or cleanup in other mock namespaces.
func (c *Client) Reconcile(ctx context.Context, owner string, desired []Mapping) error {
	return c.reconcile(ctx, owner, desired, "")
}
func (c *Client) ReconcileNamespace(ctx context.Context, owner string, desired []Mapping, namespace string) error {
	filtered := []Mapping{}
	for _, m := range desired {
		if m.Metadata["vmcp"].Namespace == namespace {
			filtered = append(filtered, m)
		}
	}
	return c.reconcile(ctx, owner, filtered, namespace)
}
func (c *Client) reconcile(ctx context.Context, owner string, desired []Mapping, namespace string) error {
	actual, err := c.List(ctx)
	if err != nil {
		return err
	}
	existing := map[string]RemoteMapping{}
	for _, m := range actual {
		existing[m.ID] = m
	}
	wanted := map[string]bool{}
	for _, m := range desired {
		wanted[m.ID] = true
	}
	failed := map[string]bool{}
	var failures []error
	for _, m := range desired {
		ns := m.Metadata["vmcp"].Namespace
		old, found := existing[m.ID]
		if found && old.Managed().Owner != owner {
			failed[ns] = true
			failures = append(failures, errors.New("managed mapping ID conflicts with an unrelated mapping"))
			continue
		}
		if found && old.Managed() == m.Metadata["vmcp"] {
			continue
		}
		method, path := "POST", "/__admin/mappings"
		if found {
			method, path = "PUT", path+"/"+m.ID
		}
		if err := c.request(ctx, method, path, m, nil); err != nil {
			failed[ns] = true
			failures = append(failures, fmt.Errorf("%s: %w", m.Name, err))
		}
	}
	for _, m := range actual {
		managed := m.Managed()
		if managed.Owner == owner && (namespace == "" || managed.Namespace == namespace) && !wanted[m.ID] && !failed[managed.Namespace] {
			if err := c.request(ctx, "DELETE", "/__admin/mappings/"+m.ID, nil, nil); err != nil {
				failures = append(failures, err)
			}
		}
	}
	return errors.Join(failures...)
}

func ValidateEndpoint(base string) error {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.User != nil || u.ForceQuery || u.RawPath != "" || strings.ContainsAny(base, "?#") || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("WireMock URL must be an HTTP(S) origin without credentials, path, query or fragment")
	}
	if err := netpolicy.ValidateURL(base); err != nil {
		return errors.New("WireMock URL is not allowed by egress policy")
	}
	return nil
}
