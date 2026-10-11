package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bishoku/vmcp/core/internal/apierror"
)

const keyPrefixLen = 12

// AgentKey is the API read model. The secret itself is never stored or returned
// again after creation or rotation.
type AgentKey struct {
	Slug           string     `json:"slug"`
	Description    string     `json:"description"`
	KeyPrefix      string     `json:"keyPrefix"`
	VirtualServers []string   `json:"virtualServers"`
	IPAllowlist    []string   `json:"ipAllowlist"`
	RateLimit      *int       `json:"rateLimitPerMinute"`
	ExpiresAt      *time.Time `json:"expiresAt"`
	RevokedAt      *time.Time `json:"revokedAt"`
	Status         string     `json:"status"` // active | expired | revoked
	Version        int64      `json:"-"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

// AgentKeyInput is the writable part of an agent key.
type AgentKeyInput struct {
	Description    string
	VirtualServers []string
	IPAllowlist    []string
	RateLimit      *int
	ExpiresAt      *time.Time
}

// AgentKeyPatch is a JSON merge patch. ExpiresAtSet distinguishes "absent" from "null" (clear).
type AgentKeyPatch struct {
	Description    *string
	VirtualServers *[]string
	IPAllowlist    *[]string
	RateLimitSet   bool
	RateLimit      *int
	ExpiresAtSet   bool
	ExpiresAt      *time.Time
}

// HashKey returns the lookup hash of a presented key.
func HashKey(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

func newSecret(label string) (secret, prefix string, err error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", err
	}
	secret = label + base64.RawURLEncoding.EncodeToString(b[:])
	return secret, secret[:keyPrefixLen], nil
}

// NormalizeIPs validates entries (address or CIDR) and returns canonical CIDR strings.
func NormalizeIPs(in []string) ([]string, []apierror.FieldError) {
	out := make([]string, 0, len(in))
	var errs []apierror.FieldError
	for i, s := range in {
		if p, err := netip.ParsePrefix(s); err == nil {
			out = append(out, p.Masked().String())
			continue
		}
		if a, err := netip.ParseAddr(s); err == nil {
			out = append(out, netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen()).String())
			continue
		}
		errs = append(errs, vf(fmt.Sprintf("/ipAllowlist/%d", i), "must be an IP address or CIDR"))
	}
	return out, errs
}

type keyRow struct {
	AgentKey
	id string
}

const keySelect = `
	SELECT k.id::text, k.slug, k.description, k.key_prefix, k.ip_allowlist, k.expires_at, k.revoked_at, k.rate_limit_per_minute,
	       k.version, k.created_at, k.updated_at
	FROM agent_keys k WHERE k.tenant_id = ` + tenantSQL

func statusOf(k *AgentKey, now time.Time) string {
	switch {
	case k.RevokedAt != nil:
		return "revoked"
	case k.ExpiresAt != nil && !k.ExpiresAt.After(now):
		return "expired"
	}
	return "active"
}

func scanKey(row scanner) (*keyRow, error) {
	var r keyRow
	if err := row.Scan(&r.id, &r.Slug, &r.Description, &r.KeyPrefix, &r.IPAllowlist, &r.ExpiresAt, &r.RevokedAt, &r.RateLimit,
		&r.Version, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	if r.IPAllowlist == nil {
		r.IPAllowlist = []string{}
	}
	r.VirtualServers = []string{}
	r.Status = statusOf(&r.AgentKey, time.Now())
	return &r, nil
}

func (t *Tx) loadKeyServers(ctx context.Context, ks []*keyRow) error {
	if len(ks) == 0 {
		return nil
	}
	byID := map[string]*keyRow{}
	ids := make([]string, 0, len(ks))
	for _, k := range ks {
		byID[k.id] = k
		ids = append(ids, k.id)
	}
	rows, err := t.q.Query(ctx, `
		SELECT s.agent_key_id::text, v.slug FROM agent_key_servers s
		JOIN virtual_servers v ON v.id = s.virtual_server_id
		WHERE s.agent_key_id = ANY($1::uuid[]) ORDER BY v.slug`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, slug string
		if err := rows.Scan(&id, &slug); err != nil {
			return err
		}
		byID[id].VirtualServers = append(byID[id].VirtualServers, slug)
	}
	return rows.Err()
}

func (t *Tx) getKey(ctx context.Context, slug string, lock bool) (*keyRow, error) {
	q := keySelect + ` AND k.slug = $1`
	if lock {
		q += ` FOR UPDATE OF k`
	}
	r, err := scanKey(t.q.QueryRow(ctx, q, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierror.NotFound("agent key", slug)
	}
	if err != nil {
		return nil, err
	}
	if err := t.loadKeyServers(ctx, []*keyRow{r}); err != nil {
		return nil, err
	}
	return r, nil
}

// AgentKeyVersion returns the ETag version of a key.
func (t *Tx) AgentKeyVersion(ctx context.Context, slug string) (int64, error) {
	r, err := t.getKey(ctx, slug, false)
	if err != nil {
		return 0, err
	}
	return r.Version, nil
}

// GetAgentKey returns one key.
func (t *Tx) GetAgentKey(ctx context.Context, slug string) (*AgentKey, error) {
	r, err := t.getKey(ctx, slug, false)
	if err != nil {
		return nil, err
	}
	return &r.AgentKey, nil
}

// ListAgentKeys pages through keys ordered by slug.
func (t *Tx) ListAgentKeys(ctx context.Context, after string, limit int) ([]AgentKey, bool, error) {
	rows, err := t.q.Query(ctx, keySelect+` AND k.slug > $1 ORDER BY k.slug LIMIT $2`, after, limit+1)
	if err != nil {
		return nil, false, err
	}
	var list []*keyRow
	for rows.Next() {
		r, err := scanKey(rows)
		if err != nil {
			rows.Close()
			return nil, false, err
		}
		list = append(list, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(list) > limit
	if more {
		list = list[:limit]
	}
	if err := t.loadKeyServers(ctx, list); err != nil {
		return nil, false, err
	}
	out := make([]AgentKey, len(list))
	for i, r := range list {
		out[i] = r.AgentKey
	}
	return out, more, nil
}

func validateKeyInput(in *AgentKeyInput) []apierror.FieldError {
	var errs []apierror.FieldError
	if len(in.Description) > 1000 {
		errs = append(errs, vf("/description", "must be at most 1000 characters"))
	}
	ips, e := NormalizeIPs(in.IPAllowlist)
	errs = append(errs, e...)
	in.IPAllowlist = ips
	errs = append(errs, validateRateLimit(in.RateLimit)...)
	if in.VirtualServers == nil {
		in.VirtualServers = []string{}
	}
	return errs
}

// checkServers reports every referenced virtual server that does not exist.
func (t *Tx) checkServers(ctx context.Context, slugs []string) ([]apierror.FieldError, error) {
	_, errs, err := t.resolveServers(ctx, slugs)
	return errs, err
}

func (t *Tx) resolveServers(ctx context.Context, slugs []string) (map[string]string, []apierror.FieldError, error) {
	rows, err := t.q.Query(ctx, `SELECT slug, id::text FROM virtual_servers WHERE tenant_id = `+tenantSQL+` AND slug = ANY($1)`, slugs)
	if err != nil {
		return nil, nil, err
	}
	ids := map[string]string{}
	for rows.Next() {
		var slug, id string
		if err := rows.Scan(&slug, &id); err != nil {
			rows.Close()
			return nil, nil, err
		}
		ids[slug] = id
	}
	rows.Close()
	var errs []apierror.FieldError
	for i, s := range slugs {
		if _, ok := ids[s]; !ok {
			errs = append(errs, fe(fmt.Sprintf("/virtualServers/%d", i), apierror.CodeUnknownVirtualServer, fmt.Sprintf("virtual server %q does not exist", s)))
		}
	}
	return ids, errs, rows.Err()
}

func (t *Tx) writeKeyServers(ctx context.Context, keyID string, slugs []string) ([]apierror.FieldError, error) {
	slugs = slices.Compact(slices.Sorted(slices.Values(slugs)))
	ids, errs, err := t.resolveServers(ctx, slugs)
	if err != nil {
		return nil, err
	}
	if len(errs) > 0 {
		return errs, nil
	}
	if _, err := t.q.Exec(ctx, `DELETE FROM agent_key_servers WHERE agent_key_id = $1::uuid`, keyID); err != nil {
		return nil, err
	}
	for _, s := range slugs {
		if _, err := t.q.Exec(ctx, `INSERT INTO agent_key_servers (agent_key_id, virtual_server_id) VALUES ($1::uuid, $2::uuid)`, keyID, ids[s]); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// CreateAgentKey creates a key and returns its secret, which is shown only once.
func (t *Tx) CreateAgentKey(ctx context.Context, slug string, in AgentKeyInput) (*AgentKey, string, error) {
	t.auditTarget("agent-key", slug)
	errs := append(SlugErrors("/slug", slug), validateKeyInput(&in)...)
	if in.ExpiresAt != nil && !in.ExpiresAt.After(time.Now()) {
		errs = append(errs, vf("/expiresAt", "must be in the future"))
	}
	srvErrs, err := t.checkServers(ctx, in.VirtualServers)
	if err != nil {
		return nil, "", err
	}
	if errs = append(errs, srvErrs...); len(errs) > 0 {
		return nil, "", apierror.Validation(errs)
	}
	secret, prefix, err := newSecret("vmcp_")
	if err != nil {
		return nil, "", err
	}
	var id string
	if err := t.q.QueryRow(ctx, `
		INSERT INTO agent_keys (tenant_id, slug, description, key_hash, key_prefix, ip_allowlist, expires_at, rate_limit_per_minute)
		VALUES (`+tenantSQL+`, $1, $2, $3, $4, $5, $6, $7) RETURNING id::text`,
		slug, in.Description, HashKey(secret), prefix, in.IPAllowlist, in.ExpiresAt, in.RateLimit).Scan(&id); err != nil {
		return nil, "", mapWriteErr(err, "agent key", slug)
	}
	if ferrs, err := t.writeKeyServers(ctx, id, in.VirtualServers); err != nil {
		return nil, "", err
	} else if len(ferrs) > 0 {
		return nil, "", apierror.Validation(ferrs)
	}
	k, err := t.GetAgentKey(ctx, slug)
	return k, secret, err
}

// PatchAgentKey applies a merge patch (secret and revocation are untouched).
func (t *Tx) PatchAgentKey(ctx context.Context, slug string, p AgentKeyPatch, ifMatch *int64) (*AgentKey, error) {
	t.auditTarget("agent-key", slug)
	cur, err := t.getKey(ctx, slug, true)
	if err != nil {
		return nil, err
	}
	if ifMatch != nil && *ifMatch != cur.Version {
		return nil, apierror.PreconditionFailed()
	}
	in := AgentKeyInput{Description: cur.Description, VirtualServers: cur.VirtualServers, IPAllowlist: cur.IPAllowlist, ExpiresAt: cur.ExpiresAt, RateLimit: cur.RateLimit}
	if p.RateLimitSet {
		in.RateLimit = p.RateLimit
	}
	if p.Description != nil {
		in.Description = *p.Description
	}
	if p.VirtualServers != nil {
		in.VirtualServers = *p.VirtualServers
	}
	if p.IPAllowlist != nil {
		in.IPAllowlist = *p.IPAllowlist
	}
	if p.ExpiresAtSet {
		in.ExpiresAt = p.ExpiresAt
		if in.ExpiresAt != nil && !in.ExpiresAt.After(time.Now()) {
			return nil, apierror.Validation([]apierror.FieldError{vf("/expiresAt", "must be in the future")})
		}
	}
	if errs := validateKeyInput(&in); len(errs) > 0 {
		return nil, apierror.Validation(errs)
	}
	serversChanged := p.VirtualServers != nil &&
		!slices.Equal(slices.Compact(slices.Sorted(slices.Values(in.VirtualServers))), cur.VirtualServers)
	sameExp := (in.ExpiresAt == nil && cur.ExpiresAt == nil) || (in.ExpiresAt != nil && cur.ExpiresAt != nil && in.ExpiresAt.Equal(*cur.ExpiresAt))
	if !serversChanged && sameExp && in.Description == cur.Description && slices.Equal(in.IPAllowlist, cur.IPAllowlist) && equalInt(in.RateLimit, cur.RateLimit) {
		return &cur.AgentKey, nil
	}
	if serversChanged {
		if ferrs, err := t.writeKeyServers(ctx, cur.id, in.VirtualServers); err != nil {
			return nil, err
		} else if len(ferrs) > 0 {
			return nil, apierror.Validation(ferrs)
		}
	}
	if _, err := t.q.Exec(ctx, `
		UPDATE agent_keys SET description=$2, ip_allowlist=$3, expires_at=$4, rate_limit_per_minute=$5, version = version + 1, updated_at = now()
		WHERE id = $1::uuid`, cur.id, in.Description, in.IPAllowlist, in.ExpiresAt, in.RateLimit); err != nil {
		return nil, err
	}
	return t.GetAgentKey(ctx, slug)
}

// RotateAgentKey issues a new secret; the old one stops working immediately.
func (t *Tx) RotateAgentKey(ctx context.Context, slug string) (*AgentKey, string, error) {
	t.auditTarget("agent-key", slug)
	cur, err := t.getKey(ctx, slug, true)
	if err != nil {
		return nil, "", err
	}
	secret, prefix, err := newSecret("vmcp_")
	if err != nil {
		return nil, "", err
	}
	if _, err := t.q.Exec(ctx, `UPDATE agent_keys SET key_hash=$2, key_prefix=$3, revoked_at=NULL, version = version + 1, updated_at = now() WHERE id = $1::uuid`,
		cur.id, HashKey(secret), prefix); err != nil {
		return nil, "", err
	}
	k, err := t.GetAgentKey(ctx, slug)
	return k, secret, err
}

// RevokeAgentKey disables a key without deleting it (idempotent).
func (t *Tx) RevokeAgentKey(ctx context.Context, slug string) (*AgentKey, error) {
	t.auditTarget("agent-key", slug)
	cur, err := t.getKey(ctx, slug, true)
	if err != nil {
		return nil, err
	}
	if cur.RevokedAt == nil {
		if _, err := t.q.Exec(ctx, `UPDATE agent_keys SET revoked_at = now(), version = version + 1, updated_at = now() WHERE id = $1::uuid`, cur.id); err != nil {
			return nil, err
		}
	}
	return t.GetAgentKey(ctx, slug)
}

// DeleteAgentKey removes a key.
func (t *Tx) DeleteAgentKey(ctx context.Context, slug string, ifMatch *int64) error {
	t.auditTarget("agent-key", slug)
	cur, err := t.getKey(ctx, slug, true)
	if err != nil {
		return err
	}
	if ifMatch != nil && *ifMatch != cur.Version {
		return apierror.PreconditionFailed()
	}
	_, err = t.q.Exec(ctx, `DELETE FROM agent_keys WHERE id = $1::uuid`, cur.id)
	return err
}
