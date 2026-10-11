package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/oauthtoken"
	"github.com/bishoku/vmcp/core/internal/tlsconf"
)

// Upstream is the API read model of a physical MCP server. Header values are never exposed.
type Upstream struct {
	Slug             string          `json:"slug"`
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	URL              string          `json:"url"`
	FallbackURLs     []string        `json:"fallbackUrls"`
	Transport        string          `json:"transport"`
	Enabled          bool            `json:"enabled"`
	HeaderNames      []string        `json:"headerNames"`
	ToolCount        int             `json:"toolCount"`
	LastRefreshAt    *time.Time      `json:"lastRefreshAt"`
	LastRefreshError *string         `json:"lastRefreshError"`
	Health           Health          `json:"health"`
	TLS              tlsconf.Info    `json:"tls"`
	Auth             oauthtoken.Info `json:"auth"`
	Version          int64           `json:"version"`
	CreatedAt        time.Time       `json:"createdAt"`
	UpdatedAt        time.Time       `json:"updatedAt"`
}

// UpstreamInput is the writable part of an upstream.
type UpstreamInput struct {
	Name         string
	Description  string
	URL          string
	FallbackURLs []string
	Transport    string
	Enabled      bool
	// Headers: nil keeps the stored headers, an empty map clears them, otherwise replaces.
	Headers map[string]string
	// TLS: nil keeps the stored settings; otherwise replaces them. An empty ClientKeyPEM
	// with a client certificate keeps the stored key.
	TLS *tlsconf.Settings
	// OAuth: nil keeps the stored client; a zero Config removes it; otherwise replaces it. An empty
	// ClientSecret keeps the stored secret.
	OAuth *oauthtoken.Config
}

// OAuthPatch is a merge patch of the OAuth client. Type "none" removes it.
type OAuthPatch struct {
	Type                                       *string
	TokenURL, ClientID, ClientSecret, Audience *string
	Scopes                                     *[]string
}

// TLSPatch is a merge patch of the TLS settings; an empty string clears a field.
type TLSPatch struct {
	CAPEM, ServerName, ClientCertPEM, ClientKeyPEM *string
}

// UpstreamPatch is a JSON merge patch. A nil header value deletes that header.
type UpstreamPatch struct {
	Name, Description, URL, Transport *string
	FallbackURLs                      *[]string
	Enabled                           *bool
	Headers                           map[string]*string
	TLS                               *TLSPatch
	OAuth                             *OAuthPatch
}

// UpstreamConn is what the control plane needs to connect to an upstream.
type UpstreamConn struct {
	URL          string
	FallbackURLs []string
	Transport    string
	Headers      map[string]string
	TLS          tlsconf.Settings
	OAuth        *oauthtoken.Config
	ToolCount    int
}

type upstreamRow struct {
	Upstream
	id      string
	headers map[string]string
	tls     tlsconf.Settings
	oauth   oauthtoken.Config
}

const upstreamSelect = `
	SELECT u.id::text, u.slug, u.name, u.description, u.url, u.transport, u.enabled, u.encrypted_headers,
	       u.version, u.created_at, u.updated_at,
	       (SELECT count(*) FROM upstream_tools t WHERE t.upstream_id = u.id),
	       s.last_refresh_at, s.last_refresh_error, u.fallback_urls, u.ca_pem, u.tls_server_name, u.client_cert_pem, u.encrypted_client_key,
	       u.oauth_token_url, u.oauth_client_id, u.oauth_scopes, u.oauth_audience, u.encrypted_oauth_secret,
	       (SELECT COALESCE(jsonb_agg(jsonb_build_object('node', h.node, 'state', h.state, 'breaker', h.breaker,
	           'url', h.url, 'latencyMs', h.latency_ms, 'error', h.error, 'checkedAt', h.checked_at)), '[]'::jsonb)
	        FROM upstream_health h WHERE h.upstream_id = u.id AND h.checked_at > now() - interval '2 minutes')
	FROM upstreams u LEFT JOIN upstream_status s ON s.upstream_id = u.id
	WHERE u.tenant_id = ` + tenantSQL

type scanner interface{ Scan(dest ...any) error }

func (t *Tx) scanUpstream(row scanner) (*upstreamRow, error) {
	var r upstreamRow
	var enc []byte
	var count int64
	var nodes, encKey, encOAuth []byte
	if err := row.Scan(&r.id, &r.Slug, &r.Name, &r.Description, &r.URL, &r.Transport, &r.Enabled, &enc,
		&r.Version, &r.CreatedAt, &r.UpdatedAt, &count, &r.LastRefreshAt, &r.LastRefreshError, &r.FallbackURLs,
		&r.tls.CAPEM, &r.tls.ServerName, &r.tls.ClientCertPEM, &encKey,
		&r.oauth.TokenURL, &r.oauth.ClientID, &r.oauth.Scopes, &r.oauth.Audience, &encOAuth, &nodes); err != nil {
		return nil, err
	}
	if len(encKey) > 0 {
		k, err := t.box.OpenBound(encKey, r.id, "upstream.client_key")
		if err != nil {
			return nil, fmt.Errorf("decrypt client key of upstream %q: %w", r.Slug, err)
		}
		r.tls.ClientKeyPEM = string(k)
	}
	if len(encOAuth) > 0 {
		k, err := t.box.OpenBound(encOAuth, r.id, "upstream.oauth_secret")
		if err != nil {
			return nil, fmt.Errorf("decrypt oauth client secret of upstream %q: %w", r.Slug, err)
		}
		r.oauth.ClientSecret = string(k)
	}
	r.Auth = r.oauth.Info()
	r.TLS = r.tls.Info()
	if r.FallbackURLs == nil {
		r.FallbackURLs = []string{}
	}
	if err := json.Unmarshal(nodes, &r.Health.Nodes); err != nil {
		return nil, err
	}
	r.Health.summarize()
	h, err := t.box.OpenHeadersBound(enc, r.id)
	if err != nil {
		return nil, fmt.Errorf("decrypt headers of upstream %q: %w", r.Slug, err)
	}
	r.headers = h
	r.HeaderNames = slices.Sorted(maps.Keys(h))
	if r.HeaderNames == nil {
		r.HeaderNames = []string{}
	}
	r.ToolCount = int(count)
	return &r, nil
}

func (t *Tx) getUpstream(ctx context.Context, slug string, lock bool) (*upstreamRow, error) {
	q := upstreamSelect + ` AND u.slug = $1`
	if lock {
		q += ` FOR UPDATE OF u`
	}
	r, err := t.scanUpstream(t.q.QueryRow(ctx, q, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierror.NotFound("upstream", slug)
	}
	return r, err
}

// UpstreamVersion returns the ETag version of an upstream.
func (t *Tx) UpstreamVersion(ctx context.Context, slug string) (int64, error) {
	r, err := t.getUpstream(ctx, slug, false)
	if err != nil {
		return 0, err
	}
	return r.Version, nil
}

// GetUpstream returns one upstream.
func (t *Tx) GetUpstream(ctx context.Context, slug string) (*Upstream, error) {
	r, err := t.getUpstream(ctx, slug, false)
	if err != nil {
		return nil, err
	}
	return &r.Upstream, nil
}

// ListUpstreams pages through upstreams ordered by slug.
func (t *Tx) ListUpstreams(ctx context.Context, after string, limit int, enabled *bool) ([]Upstream, bool, error) {
	rows, err := t.q.Query(ctx, upstreamSelect+`
		AND u.slug > $1 AND ($2::boolean IS NULL OR u.enabled = $2)
		ORDER BY u.slug LIMIT $3`, after, enabled, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []Upstream{}
	for rows.Next() {
		r, err := t.scanUpstream(rows)
		if err != nil {
			return nil, false, err
		}
		out = append(out, r.Upstream)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}

// UpstreamConnInfo returns what is needed to connect to an upstream (decrypted headers included).
func (t *Tx) UpstreamConnInfo(ctx context.Context, slug string) (*UpstreamConn, error) {
	r, err := t.getUpstream(ctx, slug, false)
	if err != nil {
		return nil, err
	}
	return &UpstreamConn{URL: r.URL, FallbackURLs: r.FallbackURLs, Transport: r.Transport, Headers: r.headers, TLS: r.tls, OAuth: r.oauthPtr(), ToolCount: r.ToolCount}, nil
}

// PutUpstream creates or replaces an upstream. With createOnly an existing slug is a 409.
// ifMatch, when set, must equal the current version.
func (t *Tx) PutUpstream(ctx context.Context, slug string, in UpstreamInput, ifMatch *int64, createOnly bool) (*Upstream, Outcome, error) {
	t.auditTarget("upstream", slug)
	if len(in.FallbackURLs) == 0 {
		in.FallbackURLs = []string{}
	}
	errs := append(SlugErrors("/slug", slug), ValidateUpstreamInput(in)...)
	if len(errs) > 0 {
		return nil, "", apierror.Validation(errs)
	}

	cur, err := t.getUpstream(ctx, slug, true)
	var nf *apierror.Error
	if errors.As(err, &nf) && nf.Code == apierror.CodeNotFound {
		if ifMatch != nil {
			return nil, "", apierror.PreconditionFailed()
		}
		var immutableID string
		if err := t.q.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&immutableID); err != nil {
			return nil, "", err
		}
		enc, err := t.box.SealHeadersBound(in.Headers, immutableID)
		if err != nil {
			return nil, "", err
		}
		tl, err := ResolveTLS(in.TLS, tlsconf.Settings{})
		if err != nil {
			return nil, "", err
		}
		encKey, err := t.sealKey(tl, immutableID)
		if err != nil {
			return nil, "", err
		}
		oc, err := ResolveOAuth(in.OAuth, oauthtoken.Config{}, in.Headers)
		if err != nil {
			return nil, "", err
		}
		encOAuth, err := t.sealOAuthSecret(oc, immutableID)
		if err != nil {
			return nil, "", err
		}
		if _, err := t.q.Exec(ctx, `
			INSERT INTO upstreams (tenant_id, slug, name, description, url, transport, enabled, encrypted_headers, fallback_urls,
			                       ca_pem, tls_server_name, client_cert_pem, encrypted_client_key,
			                       oauth_token_url, oauth_client_id, oauth_scopes, oauth_audience, encrypted_oauth_secret,id)
			VALUES (`+tenantSQL+`, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17,$18)`,
			slug, in.Name, in.Description, in.URL, in.Transport, in.Enabled, enc, in.FallbackURLs,
			tl.CAPEM, tl.ServerName, tl.ClientCertPEM, encKey,
			oc.TokenURL, oc.ClientID, nonNil(oc.Scopes), oc.Audience, encOAuth, immutableID); err != nil {
			return nil, "", mapWriteErr(err, "upstream", slug)
		}
		u, err := t.GetUpstream(ctx, slug)
		return u, Created, err
	}
	if err != nil {
		return nil, "", err
	}
	if createOnly {
		return nil, "", apierror.AlreadyExists("upstream", slug)
	}
	if ifMatch != nil && *ifMatch != cur.Version {
		return nil, "", apierror.PreconditionFailed()
	}

	setHeaders := in.Headers != nil && !maps.Equal(in.Headers, cur.headers)
	tl, err := ResolveTLS(in.TLS, cur.tls)
	if err != nil {
		return nil, "", err
	}
	setTLS := tl != cur.tls
	effHeaders := in.Headers
	if effHeaders == nil {
		effHeaders = cur.headers
	}
	oc, err := ResolveOAuth(in.OAuth, cur.oauth, effHeaders)
	if err != nil {
		return nil, "", err
	}
	setOAuth := !OAuthEqual(oc, cur.oauth)
	if in.Name == cur.Name && in.Description == cur.Description && in.URL == cur.URL && slices.Equal(in.FallbackURLs, cur.FallbackURLs) &&
		in.Transport == cur.Transport && in.Enabled == cur.Enabled && !setHeaders && !setTLS && !setOAuth {
		return &cur.Upstream, Unchanged, nil
	}
	var enc []byte
	if setHeaders {
		if enc, err = t.box.SealHeadersBound(in.Headers, cur.id); err != nil {
			return nil, "", err
		}
	}
	encKey, err := t.sealKey(tl, cur.id)
	if err != nil {
		return nil, "", err
	}
	encOAuth, err := t.sealOAuthSecret(oc, cur.id)
	if err != nil {
		return nil, "", err
	}
	if _, err := t.q.Exec(ctx, `
		UPDATE upstreams SET name=$2, description=$3, url=$4, transport=$5, enabled=$6, fallback_urls=$9,
		       encrypted_headers = CASE WHEN $7 THEN $8 ELSE encrypted_headers END,
		       ca_pem=$10, tls_server_name=$11, client_cert_pem=$12, encrypted_client_key=$13,
		       oauth_token_url=$14, oauth_client_id=$15, oauth_scopes=$16, oauth_audience=$17, encrypted_oauth_secret=$18,
		       version = version + 1, updated_at = now()
		WHERE id = $1::uuid`, cur.id, in.Name, in.Description, in.URL, in.Transport, in.Enabled, setHeaders, enc, in.FallbackURLs,
		tl.CAPEM, tl.ServerName, tl.ClientCertPEM, encKey,
		oc.TokenURL, oc.ClientID, nonNil(oc.Scopes), oc.Audience, encOAuth); err != nil {
		return nil, "", err
	}
	u, err := t.GetUpstream(ctx, slug)
	return u, Updated, err
}

// ResolveTLS returns the effective settings for a write: nil input keeps cur; an input with
// a client certificate but no key keeps the stored key. Invalid settings are a validation error.
func ResolveTLS(in *tlsconf.Settings, cur tlsconf.Settings) (tlsconf.Settings, error) {
	if in == nil {
		return cur, nil
	}
	s := *in
	if s.ClientCertPEM == "" {
		s.ClientKeyPEM = ""
	} else if s.ClientKeyPEM == "" {
		s.ClientKeyPEM = cur.ClientKeyPEM
	}
	if errs := s.Validate(); len(errs) > 0 {
		fe := make([]apierror.FieldError, len(errs))
		for i, e := range errs {
			fe[i] = apierror.FieldError{Field: "/tls/" + e.Field, Code: apierror.CodeValidationFailed, Message: e.Message}
		}
		return s, apierror.Validation(fe)
	}
	return s, nil
}

func (r *upstreamRow) oauthPtr() *oauthtoken.Config {
	if r.oauth.IsZero() {
		return nil
	}
	c := r.oauth
	return &c
}

// OAuthEqual compares two clients including the secret.
func OAuthEqual(a, b oauthtoken.Config) bool {
	return a.TokenURL == b.TokenURL && a.ClientID == b.ClientID && a.ClientSecret == b.ClientSecret &&
		a.Audience == b.Audience && slices.Equal(a.Scopes, b.Scopes)
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ResolveOAuth returns the effective OAuth client for a write: nil input keeps cur, a zero input
// removes it, an input without a secret keeps the stored secret. It rejects an invalid client and
// the combination with a static Authorization header (the two would fight over the same header).
func ResolveOAuth(in *oauthtoken.Config, cur oauthtoken.Config, headers map[string]string) (oauthtoken.Config, error) {
	c := cur
	if in != nil {
		c = *in
		if !c.IsZero() && c.ClientSecret == "" {
			c.ClientSecret = cur.ClientSecret
		}
	}
	if c.IsZero() {
		return oauthtoken.Config{}, nil
	}
	var errs []apierror.FieldError
	for _, e := range c.Validate() {
		errs = append(errs, vf("/auth/"+e.Field, e.Message))
	}
	for k := range headers {
		if strings.EqualFold(k, "Authorization") {
			errs = append(errs, vf("/auth", "cannot be combined with a static Authorization header; remove one of them"))
			break
		}
	}
	if len(errs) > 0 {
		return c, apierror.Validation(errs)
	}
	return c, nil
}

func (t *Tx) sealOAuthSecret(c oauthtoken.Config, id string) ([]byte, error) {
	if c.ClientSecret == "" {
		return nil, nil
	}
	return t.box.SealBound([]byte(c.ClientSecret), id, "upstream.oauth_secret")
}

func (t *Tx) sealKey(s tlsconf.Settings, id string) ([]byte, error) {
	if s.ClientKeyPEM == "" {
		return nil, nil
	}
	return t.box.SealBound([]byte(s.ClientKeyPEM), id, "upstream.client_key")
}

// PatchUpstream applies a JSON merge patch.
func (t *Tx) PatchUpstream(ctx context.Context, slug string, p UpstreamPatch, ifMatch *int64) (*Upstream, error) {
	t.auditTarget("upstream", slug)
	cur, err := t.getUpstream(ctx, slug, true)
	if err != nil {
		return nil, err
	}
	in := UpstreamInput{Name: cur.Name, Description: cur.Description, URL: cur.URL, FallbackURLs: cur.FallbackURLs, Transport: cur.Transport, Enabled: cur.Enabled}
	if p.FallbackURLs != nil {
		in.FallbackURLs = *p.FallbackURLs
	}
	if p.Name != nil {
		in.Name = *p.Name
	}
	if p.Description != nil {
		in.Description = *p.Description
	}
	if p.URL != nil {
		in.URL = *p.URL
	}
	if p.Transport != nil {
		in.Transport = *p.Transport
	}
	if p.Enabled != nil {
		in.Enabled = *p.Enabled
	}
	if p.TLS != nil {
		m := cur.tls
		if p.TLS.CAPEM != nil {
			m.CAPEM = *p.TLS.CAPEM
		}
		if p.TLS.ServerName != nil {
			m.ServerName = *p.TLS.ServerName
		}
		if p.TLS.ClientCertPEM != nil {
			m.ClientCertPEM = *p.TLS.ClientCertPEM
		}
		if p.TLS.ClientKeyPEM != nil {
			m.ClientKeyPEM = *p.TLS.ClientKeyPEM
		}
		in.TLS = &m
	}
	if p.OAuth != nil {
		m := cur.oauth
		if p.OAuth.Type != nil && *p.OAuth.Type == "none" {
			m = oauthtoken.Config{}
		} else {
			if p.OAuth.Type != nil && *p.OAuth.Type != oauthtoken.TypeClientCredentials {
				return nil, apierror.Validation([]apierror.FieldError{vf("/auth/type", "must be one of: none, "+oauthtoken.TypeClientCredentials)})
			}
			if p.OAuth.TokenURL != nil {
				m.TokenURL = *p.OAuth.TokenURL
			}
			if p.OAuth.ClientID != nil {
				m.ClientID = *p.OAuth.ClientID
			}
			if p.OAuth.ClientSecret != nil {
				m.ClientSecret = *p.OAuth.ClientSecret
			}
			if p.OAuth.Audience != nil {
				m.Audience = *p.OAuth.Audience
			}
			if p.OAuth.Scopes != nil {
				m.Scopes = *p.OAuth.Scopes
			}
		}
		in.OAuth = &m
	}
	if p.Headers != nil {
		merged := maps.Clone(cur.headers)
		if merged == nil {
			merged = map[string]string{}
		}
		for k, v := range p.Headers {
			if v == nil {
				delete(merged, k)
			} else {
				merged[k] = *v
			}
		}
		in.Headers = merged
	}
	u, _, err := t.PutUpstream(ctx, slug, in, ifMatch, false)
	return u, err
}

// DeleteUpstream removes an upstream. Without force it fails while virtual servers select its tools.
func (t *Tx) DeleteUpstream(ctx context.Context, slug string, force bool, ifMatch *int64) error {
	t.auditTarget("upstream", slug)
	cur, err := t.getUpstream(ctx, slug, true)
	if err != nil {
		return err
	}
	if ifMatch != nil && *ifMatch != cur.Version {
		return apierror.PreconditionFailed()
	}
	rows, err := t.q.Query(ctx, `
		SELECT DISTINCT vs.slug FROM (SELECT virtual_server_id,upstream_id FROM virtual_server_tools UNION SELECT virtual_server_id,upstream_id FROM virtual_catalog) vt
		JOIN virtual_servers vs ON vs.id = vt.virtual_server_id
		WHERE vt.upstream_id = $1::uuid ORDER BY 1`, cur.id)
	if err != nil {
		return err
	}
	var users []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return err
		}
		users = append(users, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(users) > 0 && !force {
		return apierror.InUse(fmt.Sprintf("upstream %q is selected by virtual server(s): %s; remove those selections or pass force=true",
			slug, strings.Join(users, ", ")))
	}
	_, err = t.q.Exec(ctx, `DELETE FROM upstreams WHERE id = $1::uuid`, cur.id)
	return err
}

// StoredTool is a cached upstream tool.
type StoredTool struct {
	mcp.Tool
	FetchedAt time.Time `json:"fetchedAt"`
}

// ListTools returns the cached tools of an upstream.
func (t *Tx) ListTools(ctx context.Context, slug string) ([]StoredTool, error) {
	cur, err := t.getUpstream(ctx, slug, false)
	if err != nil {
		return nil, err
	}
	rows, err := t.q.Query(ctx, `
		SELECT name, description, input_schema, annotations, fetched_at,definition
		FROM upstream_tools WHERE upstream_id = $1::uuid ORDER BY name`, cur.id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StoredTool{}
	for rows.Next() {
		var st StoredTool
		var schema, ann, definition []byte
		if err := rows.Scan(&st.Name, &st.Description, &schema, &ann, &st.FetchedAt, &definition); err != nil {
			return nil, err
		}
		st.InputSchema = json.RawMessage(schema)
		if string(ann) != "{}" && len(ann) > 0 {
			st.Annotations = json.RawMessage(ann)
		}
		st.Tool = mcp.MergeToolMetadata(st.Tool, definition)
		out = append(out, st)
	}
	return out, rows.Err()
}

// Orphan is a virtual-server selection whose tool no longer exists upstream.
type Orphan struct {
	VirtualServer string `json:"virtualServer"`
	Tool          string `json:"tool"`
}

// RefreshResult describes the effect of refreshing an upstream's tools.
type RefreshResult struct {
	ToolCount          int       `json:"toolCount"`
	Added              []string  `json:"added"`
	Removed            []string  `json:"removed"`
	Changed            []string  `json:"changed"`
	OrphanedSelections []Orphan  `json:"orphanedSelections"`
	FetchedAt          time.Time `json:"fetchedAt"`
}

// ReplaceTools makes the cached tools equal to tools. It writes only when something
// changed, so an idle refresh never bumps the config revision.
func (t *Tx) ReplaceTools(ctx context.Context, slug string, tools []mcp.Tool, at time.Time) (*RefreshResult, error) {
	t.auditTarget("upstream", slug)
	cur, err := t.getUpstream(ctx, slug, true)
	if err != nil {
		return nil, err
	}
	type cached struct {
		desc                    string
		schema, ann, definition []byte
	}
	rows, err := t.q.Query(ctx, `SELECT name, description, input_schema, annotations,definition FROM upstream_tools WHERE upstream_id = $1::uuid`, cur.id)
	if err != nil {
		return nil, err
	}
	existing := map[string]cached{}
	for rows.Next() {
		var n string
		var c cached
		if err := rows.Scan(&n, &c.desc, &c.schema, &c.ann, &c.definition); err != nil {
			rows.Close()
			return nil, err
		}
		existing[n] = c
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	res := &RefreshResult{Added: []string{}, Removed: []string{}, Changed: []string{}, OrphanedSelections: []Orphan{}, FetchedAt: at}
	seen := map[string]bool{}
	var upsert []mcp.Tool
	for _, tl := range tools {
		if tl.Name == "" || seen[tl.Name] {
			continue
		}
		seen[tl.Name] = true
		if len(tl.InputSchema) == 0 {
			tl.InputSchema = json.RawMessage(`{}`)
		}
		if len(tl.Annotations) == 0 {
			tl.Annotations = json.RawMessage(`{}`)
		}
		encoded, _ := json.Marshal(tl)
		old, ok := existing[tl.Name]
		switch {
		case !ok:
			res.Added = append(res.Added, tl.Name)
			upsert = append(upsert, tl)
		case !jsonEqual(old.definition, encoded) || old.desc != tl.Description || !jsonEqual(old.schema, tl.InputSchema) || !jsonEqual(old.ann, tl.Annotations):
			res.Changed = append(res.Changed, tl.Name)
			upsert = append(upsert, tl)
		}
	}
	for n := range existing {
		if !seen[n] {
			res.Removed = append(res.Removed, n)
		}
	}
	slices.Sort(res.Added)
	slices.Sort(res.Changed)
	slices.Sort(res.Removed)
	res.ToolCount = len(seen)

	for _, tl := range upsert {
		definition, _ := json.Marshal(tl)
		if _, err := t.q.Exec(ctx, `
			INSERT INTO upstream_tools (upstream_id, name, description, input_schema, annotations, fetched_at,definition)
			VALUES ($1::uuid, $2, $3, $4::jsonb, $5::jsonb, $6,$7::jsonb)
			ON CONFLICT (upstream_id, name) DO UPDATE
			SET description = EXCLUDED.description, input_schema = EXCLUDED.input_schema,
			    definition=EXCLUDED.definition,annotations = EXCLUDED.annotations, fetched_at = EXCLUDED.fetched_at`,
			cur.id, tl.Name, tl.Description, string(tl.InputSchema), string(tl.Annotations), at, string(definition)); err != nil {
			return nil, err
		}
	}
	if len(res.Removed) > 0 {
		if _, err := t.q.Exec(ctx, `DELETE FROM upstream_tools WHERE upstream_id = $1::uuid AND name = ANY($2)`, cur.id, res.Removed); err != nil {
			return nil, err
		}
	}

	orows, err := t.q.Query(ctx, `
		SELECT vs.slug, vt.tool_name FROM virtual_server_tools vt
		JOIN virtual_servers vs ON vs.id = vt.virtual_server_id
		WHERE vt.upstream_id = $1::uuid AND NOT EXISTS (
		    SELECT 1 FROM upstream_tools ut WHERE ut.upstream_id = vt.upstream_id AND ut.name = vt.tool_name)
		ORDER BY 1, 2`, cur.id)
	if err != nil {
		return nil, err
	}
	defer orows.Close()
	for orows.Next() {
		var o Orphan
		if err := orows.Scan(&o.VirtualServer, &o.Tool); err != nil {
			return nil, err
		}
		res.OrphanedSelections = append(res.OrphanedSelections, o)
	}
	return res, orows.Err()
}

// SetRefreshStatus records the outcome of the last refresh. It deliberately does
// not touch a table with a revision trigger.
func (t *Tx) SetRefreshStatus(ctx context.Context, slug string, at time.Time, refreshErr *string) error {
	t.auditTarget("upstream", slug)
	cur, err := t.getUpstream(ctx, slug, false)
	if err != nil {
		return err
	}
	_, err = t.q.Exec(ctx, `
		INSERT INTO upstream_status (upstream_id, last_refresh_at, last_refresh_error)
		VALUES ($1::uuid, $2, $3)
		ON CONFLICT (upstream_id) DO UPDATE SET last_refresh_at = EXCLUDED.last_refresh_at, last_refresh_error = EXCLUDED.last_refresh_error`,
		cur.id, at, refreshErr)
	return err
}

func (s StoredTool) MarshalJSON() ([]byte, error) {
	raw, err := json.Marshal(s.Tool)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	fields["fetchedAt"], _ = json.Marshal(s.FetchedAt)
	return json.Marshal(fields)
}
