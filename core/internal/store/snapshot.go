package store

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/oauthtoken"
	"github.com/bishoku/vmcp/core/internal/pii"
	"github.com/bishoku/vmcp/core/internal/policy"
	"github.com/bishoku/vmcp/core/internal/secrets"
)

// DefaultTenantSlug is the only tenant served until multi-tenancy is exposed.
const DefaultTenantSlug = "default"

// Revision returns the current config revision (cheap; used for change detection).
func Revision(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var rev int64
	err := pool.QueryRow(ctx, `SELECT revision FROM config_revision`).Scan(&rev)
	return rev, err
}

// LoadSnapshot builds a consistent snapshot of the default tenant's enabled config.
// It runs in one repeatable-read transaction so revision and content always agree.
func LoadSnapshot(ctx context.Context, pool *pgxpool.Pool, box *secrets.Box) (*config.Snapshot, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // read-only transaction

	snap := &config.Snapshot{
		Upstreams:      map[string]config.Upstream{},
		VirtualServers: map[string]config.VirtualServer{},
		AgentKeys:      map[string]config.AgentKey{},
		KeyLimits:      map[string]int{},
	}
	if err := tx.QueryRow(ctx, `SELECT revision FROM config_revision`).Scan(&snap.Revision); err != nil {
		return nil, fmt.Errorf("read revision: %w", err)
	}

	upstreamSlugs := map[string]string{} // id -> slug
	rows, err := tx.Query(ctx, `
		SELECT u.id::text, u.slug, u.url, u.fallback_urls, u.transport, u.encrypted_headers,
		       u.ca_pem, u.tls_server_name, u.client_cert_pem, u.encrypted_client_key,
		       u.oauth_token_url, u.oauth_client_id, u.oauth_scopes, u.oauth_audience, u.encrypted_oauth_secret
		FROM upstreams u JOIN tenants t ON t.id = u.tenant_id
		WHERE t.slug = $1 AND u.enabled`, DefaultTenantSlug)
	if err != nil {
		return nil, fmt.Errorf("load upstreams: %w", err)
	}
	for rows.Next() {
		var u config.Upstream
		var enc, encKey, encOAuth []byte
		var oc oauthtoken.Config
		if err := rows.Scan(&u.ID, &u.Slug, &u.URL, &u.FallbackURLs, &u.Transport, &enc,
			&u.TLS.CAPEM, &u.TLS.ServerName, &u.TLS.ClientCertPEM, &encKey,
			&oc.TokenURL, &oc.ClientID, &oc.Scopes, &oc.Audience, &encOAuth); err != nil {
			rows.Close()
			return nil, err
		}
		if u.Headers, err = box.OpenHeaders(enc); err != nil {
			rows.Close()
			return nil, fmt.Errorf("decrypt headers for upstream %q: %w", u.Slug, err)
		}
		if len(encKey) > 0 {
			k, err := box.Open(encKey)
			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("decrypt client key for upstream %q: %w", u.Slug, err)
			}
			u.TLS.ClientKeyPEM = string(k)
		}
		if oc.TokenURL != "" {
			if len(encOAuth) > 0 {
				k, err := box.Open(encOAuth)
				if err != nil {
					rows.Close()
					return nil, fmt.Errorf("decrypt oauth client secret for upstream %q: %w", u.Slug, err)
				}
				oc.ClientSecret = string(k)
			}
			u.OAuth = &oc
		}
		snap.Upstreams[u.ID] = u
		upstreamSlugs[u.ID] = u.Slug
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = tx.Query(ctx, `
		SELECT v.id::text, v.slug, v.name, v.auth_required, v.pii_config, v.tool_policy, v.rate_limit_per_minute
		FROM virtual_servers v JOIN tenants t ON t.id = v.tenant_id
		WHERE t.slug = $1 AND v.enabled`, DefaultTenantSlug)
	if err != nil {
		return nil, fmt.Errorf("load virtual servers: %w", err)
	}
	idToSlug := map[string]string{}
	policies := map[string]policy.Config{}
	for rows.Next() {
		var id string
		var v config.VirtualServer
		var piiRaw, polRaw []byte
		var rl *int
		if err := rows.Scan(&id, &v.Slug, &v.Name, &v.AuthRequired, &piiRaw, &polRaw, &rl); err != nil {
			rows.Close()
			return nil, err
		}
		if rl != nil {
			v.RateLimit = *rl
		}
		var pc pii.Config
		if err := json.Unmarshal(piiRaw, &pc); err != nil {
			rows.Close()
			return nil, fmt.Errorf("virtual server %q: pii config: %w", v.Slug, err)
		}
		if v.PII, err = pii.Compile(pc); err != nil {
			rows.Close()
			return nil, fmt.Errorf("virtual server %q: %w", v.Slug, err)
		}
		var pol policy.Config
		if err := json.Unmarshal(polRaw, &pol); err != nil {
			rows.Close()
			return nil, fmt.Errorf("virtual server %q: tool policy: %w", v.Slug, err)
		}
		policies[v.Slug] = pol
		idToSlug[id] = v.Slug
		snap.VirtualServers[v.Slug] = v
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Tools of a virtual server whose upstream is disabled are skipped.
	rows, err = tx.Query(ctx, `
		SELECT vt.virtual_server_id::text, vt.upstream_id::text, vt.tool_name, COALESCE(vt.alias, ''),
		       COALESCE(NULLIF(vt.description, ''), ut.description), ut.input_schema, ut.annotations
		FROM virtual_server_tools vt
		JOIN upstream_tools ut ON ut.upstream_id = vt.upstream_id AND ut.name = vt.tool_name
		ORDER BY vt.virtual_server_id, vt.position, vt.tool_name`)
	if err != nil {
		return nil, fmt.Errorf("load virtual tools: %w", err)
	}
	for rows.Next() {
		var vsID, upID, name, alias, desc string
		var schema, annotations []byte
		if err := rows.Scan(&vsID, &upID, &name, &alias, &desc, &schema, &annotations); err != nil {
			rows.Close()
			return nil, err
		}
		slug, ok := idToSlug[vsID]
		if !ok {
			continue // virtual server disabled or in another tenant
		}
		upSlug, ok := upstreamSlugs[upID]
		if !ok {
			continue // upstream disabled
		}
		tool := mcp.Tool{
			Name:        config.ExposedName(upSlug, name, alias),
			Description: desc,
			InputSchema: json.RawMessage(schema),
		}
		if string(annotations) != "{}" && len(annotations) > 0 {
			tool.Annotations = json.RawMessage(annotations)
		}
		vs := snap.VirtualServers[slug]
		if ok, reason := policies[slug].Decide(tool.Name, tool.Annotations); !ok {
			if vs.Blocked == nil {
				vs.Blocked = map[string]string{}
			}
			vs.Blocked[tool.Name] = reason
			snap.VirtualServers[slug] = vs
			continue
		}
		vs.Tools = append(vs.Tools, config.VirtualTool{UpstreamID: upID, UpstreamName: name, Tool: tool})
		snap.VirtualServers[slug] = vs
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := loadAgentKeys(ctx, tx, snap, idToSlug); err != nil {
		return nil, err
	}
	if err := loadBindings(ctx, tx, snap, idToSlug); err != nil {
		return nil, err
	}
	return snap, nil
}

func loadBindings(ctx context.Context, tx pgx.Tx, snap *config.Snapshot, vsSlugs map[string]string) error {
	rows, err := tx.Query(ctx, `
		SELECT b.id::text, b.slug, b.conditions, b.ip_allowlist, b.rate_limit_per_minute
		FROM identity_bindings b JOIN tenants t ON t.id = b.tenant_id
		WHERE t.slug = $1 AND b.enabled ORDER BY b.slug`, DefaultTenantSlug)
	if err != nil {
		return fmt.Errorf("load identity bindings: %w", err)
	}
	idx := map[string]int{} // binding id -> index in snap.Bindings
	for rows.Next() {
		var id string
		var raw []byte
		var ips []string
		var rl *int
		b := config.Binding{Servers: map[string]bool{}}
		if err := rows.Scan(&id, &b.Slug, &raw, &ips, &rl); err != nil {
			rows.Close()
			return err
		}
		if err := json.Unmarshal(raw, &b.Conditions); err != nil {
			rows.Close()
			return fmt.Errorf("identity binding %q: %w", b.Slug, err)
		}
		if rl != nil {
			b.RateLimit = *rl
			snap.KeyLimits[b.Principal()] = *rl
		}
		for _, s := range ips {
			if p, err := netip.ParsePrefix(s); err == nil {
				b.Allowed = append(b.Allowed, p)
			}
		}
		idx[id] = len(snap.Bindings)
		snap.Bindings = append(snap.Bindings, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	srows, err := tx.Query(ctx, `SELECT binding_id::text, virtual_server_id::text FROM identity_binding_servers`)
	if err != nil {
		return fmt.Errorf("load identity binding servers: %w", err)
	}
	defer srows.Close()
	for srows.Next() {
		var bid, vsID string
		if err := srows.Scan(&bid, &vsID); err != nil {
			return err
		}
		i, ok := idx[bid]
		slug, ok2 := vsSlugs[vsID]
		if ok && ok2 {
			snap.Bindings[i].Servers[slug] = true
		}
	}
	return srows.Err()
}

func loadAgentKeys(ctx context.Context, tx pgx.Tx, snap *config.Snapshot, vsSlugs map[string]string) error {
	rows, err := tx.Query(ctx, `
		SELECT k.id::text, k.slug, k.key_hash, k.ip_allowlist, k.expires_at, k.rate_limit_per_minute
		FROM agent_keys k JOIN tenants t ON t.id = k.tenant_id
		WHERE t.slug = $1 AND k.revoked_at IS NULL`, DefaultTenantSlug)
	if err != nil {
		return fmt.Errorf("load agent keys: %w", err)
	}
	byID := map[string]string{} // key id -> hash
	for rows.Next() {
		var id string
		var hash []byte
		var ips []string
		var rl *int
		k := config.AgentKey{Servers: map[string]bool{}}
		if err := rows.Scan(&id, &k.Slug, &hash, &ips, &k.ExpiresAt, &rl); err != nil {
			rows.Close()
			return err
		}
		if rl != nil {
			k.RateLimit = *rl
			snap.KeyLimits[k.Slug] = *rl
		}
		for _, s := range ips {
			if p, err := netip.ParsePrefix(s); err == nil {
				k.Allowed = append(k.Allowed, p)
			}
		}
		byID[id] = string(hash)
		snap.AgentKeys[string(hash)] = k
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	rows, err = tx.Query(ctx, `SELECT agent_key_id::text, virtual_server_id::text FROM agent_key_servers`)
	if err != nil {
		return fmt.Errorf("load agent key servers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var keyID, vsID string
		if err := rows.Scan(&keyID, &vsID); err != nil {
			return err
		}
		hash, ok := byID[keyID]
		slug, ok2 := vsSlugs[vsID]
		if ok && ok2 {
			snap.AgentKeys[hash].Servers[slug] = true
		}
	}
	return rows.Err()
}
