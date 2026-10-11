package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bishoku/vmcp/core/internal/apierror"
)

// Admin token scopes. Fine-grained RBAC arrives with SSO in a later phase.
const (
	ScopeAdmin = "admin" // full access
	ScopeRead  = "read"  // GET only
)

// AdminToken is the API read model; the secret is never returned after creation.
type AdminToken struct {
	Slug        string     `json:"slug"`
	Description string     `json:"description"`
	Scope       string     `json:"scope"`
	TokenPrefix string     `json:"tokenPrefix"`
	ExpiresAt   *time.Time `json:"expiresAt"`
	RevokedAt   *time.Time `json:"revokedAt"`
	LastUsedAt  *time.Time `json:"lastUsedAt"`
	Status      string     `json:"status"`
	Version     int64      `json:"version"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// AdminTokenInput is the writable part of a token.
type AdminTokenInput struct {
	Description string
	Scope       string
	ExpiresAt   *time.Time
}

// AdminTokenPatch is a merge patch; ExpiresAtSet separates "absent" from "null".
type AdminTokenPatch struct {
	Description  *string
	Scope        *string
	ExpiresAtSet bool
	ExpiresAt    *time.Time
}

const adminSelect = `
	SELECT slug, description, scope, token_prefix, expires_at, revoked_at, last_used_at, version, created_at, updated_at
	FROM admin_tokens WHERE tenant_id = ` + tenantSQL

func scanAdmin(row scanner) (*AdminToken, error) {
	var a AdminToken
	if err := row.Scan(&a.Slug, &a.Description, &a.Scope, &a.TokenPrefix, &a.ExpiresAt, &a.RevokedAt, &a.LastUsedAt,
		&a.Version, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return nil, err
	}
	now := time.Now()
	switch {
	case a.RevokedAt != nil:
		a.Status = "revoked"
	case a.ExpiresAt != nil && !a.ExpiresAt.After(now):
		a.Status = "expired"
	default:
		a.Status = "active"
	}
	return &a, nil
}

func validateAdminInput(in AdminTokenInput) []apierrorField {
	var errs []apierrorField
	if in.Scope != ScopeAdmin && in.Scope != ScopeRead {
		errs = append(errs, vf("/scope", "must be one of: admin, read"))
	}
	if len(in.Description) > 1000 {
		errs = append(errs, vf("/description", "must be at most 1000 characters"))
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.After(time.Now()) {
		errs = append(errs, vf("/expiresAt", "must be in the future"))
	}
	return errs
}

type apierrorField = apierror.FieldError

// GetAdminToken returns one token.
func (t *Tx) GetAdminToken(ctx context.Context, slug string) (*AdminToken, error) {
	a, err := scanAdmin(t.q.QueryRow(ctx, adminSelect+` AND slug = $1`, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierror.NotFound("admin token", slug)
	}
	return a, err
}

// AdminTokenVersion returns the ETag version.
func (t *Tx) AdminTokenVersion(ctx context.Context, slug string) (int64, error) {
	a, err := t.GetAdminToken(ctx, slug)
	if err != nil {
		return 0, err
	}
	return a.Version, nil
}

// ListAdminTokens pages through tokens ordered by slug.
func (t *Tx) ListAdminTokens(ctx context.Context, after string, limit int) ([]AdminToken, bool, error) {
	rows, err := t.q.Query(ctx, adminSelect+` AND slug > $1 ORDER BY slug LIMIT $2`, after, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []AdminToken{}
	for rows.Next() {
		a, err := scanAdmin(rows)
		if err != nil {
			return nil, false, err
		}
		out = append(out, *a)
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

// CreateAdminToken creates a token and returns its secret (shown once).
func (t *Tx) CreateAdminToken(ctx context.Context, slug string, in AdminTokenInput) (*AdminToken, string, error) {
	t.auditTarget("admin-token", slug)
	if in.Scope == "" {
		in.Scope = ScopeRead
	}
	if errs := append(SlugErrors("/slug", slug), validateAdminInput(in)...); len(errs) > 0 {
		return nil, "", apierror.Validation(errs)
	}
	secret, prefix, err := newSecret("vmcpa_")
	if err != nil {
		return nil, "", err
	}
	if _, err := t.q.Exec(ctx, `
		INSERT INTO admin_tokens (tenant_id, slug, description, scope, token_hash, token_prefix, expires_at)
		VALUES (`+tenantSQL+`, $1, $2, $3, $4, $5, $6)`,
		slug, in.Description, in.Scope, HashKey(secret), prefix, in.ExpiresAt); err != nil {
		return nil, "", mapWriteErr(err, "admin token", slug)
	}
	a, err := t.GetAdminToken(ctx, slug)
	return a, secret, err
}

// PatchAdminToken applies a merge patch.
func (t *Tx) PatchAdminToken(ctx context.Context, slug string, p AdminTokenPatch, ifMatch *int64) (*AdminToken, error) {
	t.auditTarget("admin-token", slug)
	cur, err := scanAdmin(t.q.QueryRow(ctx, adminSelect+` AND slug = $1 FOR UPDATE`, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierror.NotFound("admin token", slug)
	}
	if err != nil {
		return nil, err
	}
	if ifMatch != nil && *ifMatch != cur.Version {
		return nil, apierror.PreconditionFailed()
	}
	in := AdminTokenInput{Description: cur.Description, Scope: cur.Scope, ExpiresAt: cur.ExpiresAt}
	if p.Description != nil {
		in.Description = *p.Description
	}
	if p.Scope != nil {
		in.Scope = *p.Scope
	}
	if p.ExpiresAtSet {
		in.ExpiresAt = p.ExpiresAt
	}
	// Only a changed expiry must lie in the future; an unchanged past one stays valid input.
	check := in
	if !p.ExpiresAtSet {
		check.ExpiresAt = nil
	}
	if errs := validateAdminInput(check); len(errs) > 0 {
		return nil, apierror.Validation(errs)
	}
	if _, err := t.q.Exec(ctx, `UPDATE admin_tokens SET description=$2, scope=$3, expires_at=$4, version = version + 1, updated_at = now()
		WHERE tenant_id = `+tenantSQL+` AND slug = $1`, slug, in.Description, in.Scope, in.ExpiresAt); err != nil {
		return nil, err
	}
	return t.GetAdminToken(ctx, slug)
}

// RotateAdminToken issues a new secret and clears any revocation.
func (t *Tx) RotateAdminToken(ctx context.Context, slug string, matches ...*int64) (*AdminToken, string, error) {
	t.auditTarget("admin-token", slug)
	if len(matches) > 0 && matches[0] != nil {
		if err := t.lockAdminVersion(ctx, slug, matches[0]); err != nil {
			return nil, "", err
		}
	}

	secret, prefix, err := newSecret("vmcpa_")
	if err != nil {
		return nil, "", err
	}
	tag, err := t.q.Exec(ctx, `UPDATE admin_tokens SET token_hash=$2, token_prefix=$3, revoked_at=NULL, version = version + 1, updated_at = now()
		WHERE tenant_id = `+tenantSQL+` AND slug = $1`, slug, HashKey(secret), prefix)
	if err != nil {
		return nil, "", err
	}
	if tag.RowsAffected() == 0 {
		return nil, "", apierror.NotFound("admin token", slug)
	}
	a, err := t.GetAdminToken(ctx, slug)
	return a, secret, err
}

// RevokeAdminToken disables a token (idempotent).
func (t *Tx) RevokeAdminToken(ctx context.Context, slug string, matches ...*int64) (*AdminToken, error) {
	t.auditTarget("admin-token", slug)
	if len(matches) > 0 && matches[0] != nil {
		if err := t.lockAdminVersion(ctx, slug, matches[0]); err != nil {
			return nil, err
		}
	}

	tag, err := t.q.Exec(ctx, `UPDATE admin_tokens SET revoked_at = COALESCE(revoked_at, now()),
		version = CASE WHEN revoked_at IS NULL THEN version + 1 ELSE version END, updated_at = now()
		WHERE tenant_id = `+tenantSQL+` AND slug = $1`, slug)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, apierror.NotFound("admin token", slug)
	}
	return t.GetAdminToken(ctx, slug)
}

// DeleteAdminToken removes a token.
func (t *Tx) DeleteAdminToken(ctx context.Context, slug string, ifMatch *int64) error {
	t.auditTarget("admin-token", slug)
	if err := t.lockAdminVersion(ctx, slug, ifMatch); err != nil {
		return err
	}

	tag, err := t.q.Exec(ctx, `DELETE FROM admin_tokens WHERE tenant_id = `+tenantSQL+` AND slug = $1`, slug)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("admin token", slug)
	}
	return nil
}

// LookupAdminToken resolves a presented secret to its scope. ok is false for unknown,
// revoked or expired tokens. last_used_at is refreshed at most once a minute.
func (s *Store) LookupAdminIdentity(ctx context.Context, secret string) (scope, id, slug string, ok bool, err error) {
	err = s.Pool.QueryRow(ctx, `
		SELECT id::text, scope, slug FROM admin_tokens
		WHERE token_hash = $1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now())`,
		HashKey(secret)).Scan(&id, &scope, &slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", false, nil
	}
	if err != nil {
		return "", "", "", false, err
	}
	_, _ = s.Pool.Exec(ctx, `UPDATE admin_tokens SET last_used_at = now()
		WHERE id = $1::uuid AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute')`, id)
	return scope, id, slug, true, nil
}

// LookupAdminToken is the compatibility scope-only view.
func (s *Store) LookupAdminToken(ctx context.Context, secret string) (string, bool, error) {
	scope, _, _, ok, err := s.LookupAdminIdentity(ctx, secret)
	return scope, ok, err
}

func (t *Tx) lockAdminVersion(ctx context.Context, slug string, match *int64) error {
	if match == nil {
		return nil
	}
	cur, err := scanAdmin(t.q.QueryRow(ctx, adminSelect+` AND slug=$1 FOR UPDATE`, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return apierror.NotFound("admin token", slug)
	}
	if err != nil {
		return err
	}
	if cur.Version != *match {
		return apierror.PreconditionFailed()
	}
	return nil
}
