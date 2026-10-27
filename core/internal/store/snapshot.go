package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"net/netip"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/oauthtoken"
	"github.com/bishoku/vmcp/core/internal/pii"
	"github.com/bishoku/vmcp/core/internal/policy"
	"github.com/bishoku/vmcp/core/internal/restapi"
	"github.com/bishoku/vmcp/core/internal/secrets"
)

type secretCacheKey struct {
	boxHash [32]byte
	id      string
	purpose string
	encHash [32]byte
}

var (
	snapshotCacheMu sync.RWMutex
	piiCache        = map[[32]byte]*pii.Masker{}
	headersCache    = map[secretCacheKey]map[string]string{}
	secretCache     = map[secretCacheKey]string{}
)

func cachedCompilePII(raw []byte) (*pii.Masker, error) {
	key := sha256.Sum256(raw)
	snapshotCacheMu.RLock()
	m, ok := piiCache[key]
	snapshotCacheMu.RUnlock()
	if ok {
		return m, nil
	}
	var pc pii.Config
	if err := json.Unmarshal(raw, &pc); err != nil {
		return nil, fmt.Errorf("pii config: %w", err)
	}
	compiled, err := pii.Compile(pc)
	if err != nil {
		return nil, err
	}
	snapshotCacheMu.Lock()
	if len(piiCache) >= 2048 {
		clear(piiCache)
	}
	piiCache[key] = compiled
	snapshotCacheMu.Unlock()
	return compiled, nil
}

func cachedOpenHeaders(box *secrets.Box, boxTag [32]byte, enc []byte, id string) (map[string]string, error) {
	if len(enc) == 0 {
		return map[string]string{}, nil
	}
	ck := secretCacheKey{boxHash: boxTag, id: id, purpose: "headers", encHash: sha256.Sum256(enc)}
	snapshotCacheMu.RLock()
	cached, ok := headersCache[ck]
	snapshotCacheMu.RUnlock()
	if ok {
		return maps.Clone(cached), nil
	}
	hdrs, err := box.OpenHeadersBound(enc, id)
	if err != nil {
		return nil, err
	}
	snapshotCacheMu.Lock()
	if len(headersCache) >= 2048 {
		clear(headersCache)
	}
	headersCache[ck] = maps.Clone(hdrs)
	snapshotCacheMu.Unlock()
	return hdrs, nil
}

func cachedOpenBound(box *secrets.Box, boxTag [32]byte, enc []byte, id, purpose string) (string, error) {
	ck := secretCacheKey{boxHash: boxTag, id: id, purpose: purpose, encHash: sha256.Sum256(enc)}
	snapshotCacheMu.RLock()
	val, ok := secretCache[ck]
	snapshotCacheMu.RUnlock()
	if ok {
		return val, nil
	}
	raw, err := box.OpenBound(enc, id, purpose)
	if err != nil {
		return "", err
	}
	val = string(raw)
	snapshotCacheMu.Lock()
	if len(secretCache) >= 2048 {
		clear(secretCache)
	}
	secretCache[ck] = val
	snapshotCacheMu.Unlock()
	return val, nil
}

// DefaultTenantSlug is the internal configuration row within a dedicated tenant database.
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
	return LoadSnapshotWithPrevious(ctx, pool, box, nil)
}

// LoadSnapshotWithPrevious reuses immutable REST schemas only when their complete
// source, base URL and common headers match. Fresh database reads and bound secret
// decryption still happen before reuse; authorization and revision checks are unchanged.
// Retention is bounded by the current/previous snapshots, with no global schema cache.
func LoadSnapshotWithPrevious(ctx context.Context, pool *pgxpool.Pool, box *secrets.Box, previous *config.Snapshot) (*config.Snapshot, error) {
	boxTag := sha256.Sum256([]byte(fmt.Sprintf("%p", box)))
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
		       u.oauth_token_url, u.oauth_client_id, u.oauth_scopes, u.oauth_audience, u.encrypted_oauth_secret, u.discovered_instructions,u.kind,u.encrypted_rest_config,u.tenant_id::text,u.caller_context,u.request_header_mappings
		FROM upstreams u JOIN tenants t ON t.id = u.tenant_id
		WHERE t.slug = $1 AND u.enabled`, DefaultTenantSlug)
	if err != nil {
		return nil, fmt.Errorf("load upstreams: %w", err)
	}
	for rows.Next() {
		var u config.Upstream
		var enc, encKey, encOAuth, encREST []byte
		var oc oauthtoken.Config
		if err := rows.Scan(&u.ID, &u.Slug, &u.URL, &u.FallbackURLs, &u.Transport, &enc,
			&u.TLS.CAPEM, &u.TLS.ServerName, &u.TLS.ClientCertPEM, &encKey,
			&oc.TokenURL, &oc.ClientID, &oc.Scopes, &oc.Audience, &encOAuth, &u.Instructions, &u.Kind, &encREST, &u.Tenant, &u.CallerContext, &u.RequestHeaderMappings); err != nil {
			rows.Close()
			return nil, err
		}
		if u.Headers, err = cachedOpenHeaders(box, boxTag, enc, u.ID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("decrypt headers for upstream %q: %w", u.Slug, err)
		}
		if len(encKey) > 0 {
			k, err := cachedOpenBound(box, boxTag, encKey, u.ID, "upstream.client_key")
			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("decrypt client key for upstream %q: %w", u.Slug, err)
			}
			u.TLS.ClientKeyPEM = k
		}
		if oc.TokenURL != "" {
			if len(encOAuth) > 0 {
				k, err := cachedOpenBound(box, boxTag, encOAuth, u.ID, "upstream.oauth_secret")
				if err != nil {
					rows.Close()
					return nil, fmt.Errorf("decrypt oauth client secret for upstream %q: %w", u.Slug, err)
				}
				oc.ClientSecret = k
			}
			u.OAuth = &oc
		}
		if u.Kind == "rest" {
			raw, err := cachedOpenBound(box, boxTag, encREST, u.ID, "upstream.rest_config")
			if err != nil {
				rows.Close()
				return nil, err
			}
			var rest restapi.Config
			if err = json.Unmarshal([]byte(raw), &rest); err != nil {
				rows.Close()
				return nil, err
			}
			u.RESTSourceHash = sha256.Sum256([]byte(raw))
			if previous != nil {
				old := previous.Upstreams[u.ID]
				if old.REST != nil && old.Tenant == u.Tenant && old.RESTSourceHash == u.RESTSourceHash && old.URL == u.URL && maps.Equal(old.Headers, u.Headers) {
					u.REST = old.REST
				}
			}
			if u.REST == nil {
				u.REST, err = restapi.Compile(u.URL, rest, u.Headers)
			}
			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("compile REST upstream %s: %w", u.Slug, err)
			}
			u.OAuth = rest.Auth.OAuth()
		}
		snap.Upstreams[u.ID] = u.WithPrecomputedFingerprint()
		upstreamSlugs[u.ID] = u.Slug
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = tx.Query(ctx, `
		SELECT v.id::text, v.slug, v.name, v.auth_required, v.pii_config, v.tool_policy,v.client_features, v.rate_limit_per_minute, v.guidance
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
		var piiRaw, polRaw, features, guidance []byte
		var rl *int
		if err := rows.Scan(&id, &v.Slug, &v.Name, &v.AuthRequired, &piiRaw, &polRaw, &features, &rl, &guidance); err != nil {
			rows.Close()
			return nil, err
		}
		if err := json.Unmarshal(guidance, &v.Guidance); err != nil {
			rows.Close()
			return nil, err
		}
		if rl != nil {
			v.RateLimit = *rl
		}
		if err := json.Unmarshal(features, &v.ClientFeatures); err != nil {
			rows.Close()
			return nil, err
		}
		if v.PII, err = cachedCompilePII(piiRaw); err != nil {
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
		       COALESCE(NULLIF(vt.description, ''), ut.description), ut.input_schema, ut.annotations,ut.definition
		FROM virtual_server_tools vt
		JOIN upstream_tools ut ON ut.upstream_id = vt.upstream_id AND ut.name = vt.tool_name
		ORDER BY vt.virtual_server_id, vt.position, vt.tool_name`)
	if err != nil {
		return nil, fmt.Errorf("load virtual tools: %w", err)
	}
	for rows.Next() {
		var vsID, upID, name, alias, desc string
		var schema, annotations, definition []byte
		if err := rows.Scan(&vsID, &upID, &name, &alias, &desc, &schema, &annotations, &definition); err != nil {
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
		tool = mcp.MergeToolMetadata(tool, definition)
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
	for slug, vs := range snap.VirtualServers {
		snap.VirtualServers[slug] = vs.WithToolIndex()
	}
	if err := loadVirtualCatalog(ctx, tx, snap, idToSlug); err != nil {
		return nil, err
	}
	if err := applyOwnership(ctx, tx, snap); err != nil {
		return nil, err
	}
	for slug, vs := range snap.VirtualServers {
		snap.VirtualServers[slug] = vs.WithGuidance(snap.Upstreams).WithPreparedMetadata()
	}
	if err := loadAgentKeys(ctx, tx, snap, idToSlug); err != nil {
		return nil, err
	}
	if err := loadBindings(ctx, tx, snap, idToSlug); err != nil {
		return nil, err
	}
	snap.Audit, err = loadAuditSettings(ctx, tx)
	if err != nil {
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
		b.ID = id
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
		k.ID = id
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
