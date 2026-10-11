package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bishoku/vmcp/core/internal/agentauth"
	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/config"
)

// Binding is the API read model of an identity binding.
type Binding struct {
	Slug           string             `json:"slug"`
	Description    string             `json:"description"`
	Conditions     []config.Condition `json:"conditions"`
	VirtualServers []string           `json:"virtualServers"`
	IPAllowlist    []string           `json:"ipAllowlist"`
	RateLimit      *int               `json:"rateLimitPerMinute"`
	Enabled        bool               `json:"enabled"`
	Version        int64              `json:"-"`
	CreatedAt      time.Time          `json:"createdAt"`
	UpdatedAt      time.Time          `json:"updatedAt"`
}

// BindingInput is the writable part of a binding.
type BindingInput struct {
	Description    string
	Conditions     []config.Condition
	VirtualServers []string
	IPAllowlist    []string
	RateLimit      *int
	Enabled        bool
}

// BindingPatch is a JSON merge patch.
type BindingPatch struct {
	Description    *string
	Conditions     *[]config.Condition
	VirtualServers *[]string
	IPAllowlist    *[]string
	RateLimitSet   bool
	RateLimit      *int
	Enabled        *bool
}

type bindingRow struct {
	Binding
	id string
}

const bindingSelect = `
	SELECT b.id::text, b.slug, b.description, b.conditions, b.ip_allowlist, b.rate_limit_per_minute, b.enabled,
	       b.version, b.created_at, b.updated_at
	FROM identity_bindings b WHERE b.tenant_id = ` + tenantSQL

func scanBinding(row scanner) (*bindingRow, error) {
	var r bindingRow
	var raw []byte
	if err := row.Scan(&r.id, &r.Slug, &r.Description, &raw, &r.IPAllowlist, &r.RateLimit, &r.Enabled, &r.Version, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &r.Conditions); err != nil {
		return nil, err
	}
	if r.IPAllowlist == nil {
		r.IPAllowlist = []string{}
	}
	r.VirtualServers = []string{}
	return &r, nil
}

func (t *Tx) loadBindingServers(ctx context.Context, bs []*bindingRow) error {
	if len(bs) == 0 {
		return nil
	}
	byID := map[string]*bindingRow{}
	ids := make([]string, 0, len(bs))
	for _, b := range bs {
		byID[b.id] = b
		ids = append(ids, b.id)
	}
	rows, err := t.q.Query(ctx, `
		SELECT s.binding_id::text, v.slug FROM identity_binding_servers s
		JOIN virtual_servers v ON v.id = s.virtual_server_id
		WHERE s.binding_id = ANY($1::uuid[]) ORDER BY v.slug`, ids)
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

func (t *Tx) getBinding(ctx context.Context, slug string, lock bool) (*bindingRow, error) {
	q := bindingSelect + ` AND b.slug = $1`
	if lock {
		q += ` FOR UPDATE OF b`
	}
	r, err := scanBinding(t.q.QueryRow(ctx, q, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierror.NotFound("identity binding", slug)
	}
	if err != nil {
		return nil, err
	}
	return r, t.loadBindingServers(ctx, []*bindingRow{r})
}

// BindingVersion returns the ETag version of a binding.
func (t *Tx) BindingVersion(ctx context.Context, slug string) (int64, error) {
	r, err := t.getBinding(ctx, slug, false)
	if err != nil {
		return 0, err
	}
	return r.Version, nil
}

// GetBinding returns one binding.
func (t *Tx) GetBinding(ctx context.Context, slug string) (*Binding, error) {
	r, err := t.getBinding(ctx, slug, false)
	if err != nil {
		return nil, err
	}
	return &r.Binding, nil
}

// ListBindings pages through bindings ordered by slug.
func (t *Tx) ListBindings(ctx context.Context, after string, limit int) ([]Binding, bool, error) {
	rows, err := t.q.Query(ctx, bindingSelect+` AND b.slug > $1 ORDER BY b.slug LIMIT $2`, after, limit+1)
	if err != nil {
		return nil, false, err
	}
	var list []*bindingRow
	for rows.Next() {
		r, err := scanBinding(rows)
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
	if err := t.loadBindingServers(ctx, list); err != nil {
		return nil, false, err
	}
	out := make([]Binding, len(list))
	for i, r := range list {
		out[i] = r.Binding
	}
	return out, more, nil
}

func validateBindingInput(in *BindingInput) []apierror.FieldError {
	var errs []apierror.FieldError
	if len(in.Description) > 1000 {
		errs = append(errs, vf("/description", "must be at most 1000 characters"))
	}
	if len(in.Conditions) == 0 || len(in.Conditions) > 20 {
		errs = append(errs, vf("/conditions", "must contain 1 to 20 conditions"))
	}
	for i, c := range in.Conditions {
		p := fmt.Sprintf("/conditions/%d", i)
		if c.Claim == "" || len(c.Claim) > 200 || strings.ContainsAny(c.Claim, " \t\n") {
			errs = append(errs, vf(p+"/claim", "must be a claim name or dotted path without spaces (at most 200 characters)"))
		}
		switch c.Op {
		case agentauth.OpEquals, agentauth.OpContains:
			if c.Value == "" || len(c.Value) > 500 {
				errs = append(errs, vf(p+"/value", "is required (at most 500 characters)"))
			}
		case agentauth.OpExists:
			if c.Value != "" {
				errs = append(errs, vf(p+"/value", "must be empty for op exists"))
			}
		default:
			errs = append(errs, vf(p+"/op", "must be one of: equals, contains, exists"))
		}
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

func (t *Tx) writeBindingServers(ctx context.Context, bindingID string, slugs []string) ([]apierror.FieldError, error) {
	slugs = slices.Compact(slices.Sorted(slices.Values(slugs)))
	ids, errs, err := t.resolveServers(ctx, slugs)
	if err != nil || len(errs) > 0 {
		return errs, err
	}
	if _, err := t.q.Exec(ctx, `DELETE FROM identity_binding_servers WHERE binding_id = $1::uuid`, bindingID); err != nil {
		return nil, err
	}
	for _, s := range slugs {
		if _, err := t.q.Exec(ctx, `INSERT INTO identity_binding_servers (binding_id, virtual_server_id) VALUES ($1::uuid, $2::uuid)`, bindingID, ids[s]); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// PutBinding creates or replaces a binding. With createOnly an existing slug is a 409.
func (t *Tx) PutBinding(ctx context.Context, slug string, in BindingInput, ifMatch *int64, createOnly bool) (*Binding, Outcome, error) {
	t.auditTarget("identity-binding", slug)
	errs := append(SlugErrors("/slug", slug), validateBindingInput(&in)...)
	if len(errs) > 0 {
		return nil, "", apierror.Validation(errs)
	}
	raw, _ := json.Marshal(in.Conditions)
	cur, err := t.getBinding(ctx, slug, true)
	var nf *apierror.Error
	if errors.As(err, &nf) && nf.Code == apierror.CodeNotFound {
		if ifMatch != nil {
			return nil, "", apierror.PreconditionFailed()
		}
		var id string
		if err := t.q.QueryRow(ctx, `
			INSERT INTO identity_bindings (tenant_id, slug, description, conditions, ip_allowlist, rate_limit_per_minute, enabled)
			VALUES (`+tenantSQL+`, $1, $2, $3::jsonb, $4, $5, $6) RETURNING id::text`,
			slug, in.Description, string(raw), in.IPAllowlist, in.RateLimit, in.Enabled).Scan(&id); err != nil {
			return nil, "", mapWriteErr(err, "identity binding", slug)
		}
		if ferrs, err := t.writeBindingServers(ctx, id, in.VirtualServers); err != nil {
			return nil, "", err
		} else if len(ferrs) > 0 {
			return nil, "", apierror.Validation(ferrs)
		}
		b, err := t.GetBinding(ctx, slug)
		return b, Created, err
	}
	if err != nil {
		return nil, "", err
	}
	if createOnly {
		return nil, "", apierror.AlreadyExists("identity binding", slug)
	}
	if ifMatch != nil && *ifMatch != cur.Version {
		return nil, "", apierror.PreconditionFailed()
	}
	wantServers := slices.Compact(slices.Sorted(slices.Values(in.VirtualServers)))
	serversSame := slices.Equal(wantServers, cur.VirtualServers)
	if serversSame && in.Description == cur.Description && slices.Equal(in.Conditions, cur.Conditions) &&
		slices.Equal(in.IPAllowlist, cur.IPAllowlist) && equalInt(in.RateLimit, cur.RateLimit) && in.Enabled == cur.Enabled {
		return &cur.Binding, Unchanged, nil
	}
	if !serversSame {
		if ferrs, err := t.writeBindingServers(ctx, cur.id, in.VirtualServers); err != nil {
			return nil, "", err
		} else if len(ferrs) > 0 {
			return nil, "", apierror.Validation(ferrs)
		}
	}
	if _, err := t.q.Exec(ctx, `
		UPDATE identity_bindings SET description=$2, conditions=$3::jsonb, ip_allowlist=$4, rate_limit_per_minute=$5, enabled=$6,
		       version = version + 1, updated_at = now()
		WHERE id = $1::uuid`, cur.id, in.Description, string(raw), in.IPAllowlist, in.RateLimit, in.Enabled); err != nil {
		return nil, "", err
	}
	b, err := t.GetBinding(ctx, slug)
	return b, Updated, err
}

// PatchBinding applies a JSON merge patch.
func (t *Tx) PatchBinding(ctx context.Context, slug string, p BindingPatch, ifMatch *int64) (*Binding, error) {
	t.auditTarget("identity-binding", slug)
	cur, err := t.getBinding(ctx, slug, true)
	if err != nil {
		return nil, err
	}
	in := BindingInput{Description: cur.Description, Conditions: cur.Conditions, VirtualServers: cur.VirtualServers,
		IPAllowlist: cur.IPAllowlist, RateLimit: cur.RateLimit, Enabled: cur.Enabled}
	if p.Description != nil {
		in.Description = *p.Description
	}
	if p.Conditions != nil {
		in.Conditions = *p.Conditions
	}
	if p.VirtualServers != nil {
		in.VirtualServers = *p.VirtualServers
	}
	if p.IPAllowlist != nil {
		in.IPAllowlist = *p.IPAllowlist
	}
	if p.RateLimitSet {
		in.RateLimit = p.RateLimit
	}
	if p.Enabled != nil {
		in.Enabled = *p.Enabled
	}
	b, _, err := t.PutBinding(ctx, slug, in, ifMatch, false)
	return b, err
}

// DeleteBinding removes a binding.
func (t *Tx) DeleteBinding(ctx context.Context, slug string, ifMatch *int64) error {
	t.auditTarget("identity-binding", slug)
	cur, err := t.getBinding(ctx, slug, true)
	if err != nil {
		return err
	}
	if ifMatch != nil && *ifMatch != cur.Version {
		return apierror.PreconditionFailed()
	}
	_, err = t.q.Exec(ctx, `DELETE FROM identity_bindings WHERE id = $1::uuid`, cur.id)
	return err
}
