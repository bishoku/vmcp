package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/pii"
	"github.com/bishoku/vmcp/core/internal/policy"
)

// ToolSelection picks one upstream tool for a virtual server.
type ToolSelection struct {
	Upstream string `json:"upstream"`
	Tool     string `json:"tool"`
	Alias    string `json:"alias,omitempty"`
	// Description replaces the upstream description in tools/list; empty uses the upstream text.
	Description string `json:"description,omitempty"`
}

// ResolvedSelection adds the name agents see.
type ResolvedSelection struct {
	ToolSelection
	ExposedName string `json:"exposedName"`
	// UpstreamDescription is the current upstream text; DescriptionDrift is true when an override
	// exists and the upstream text changed since the override was written.
	UpstreamDescription string `json:"upstreamDescription"`
	DescriptionDrift    bool   `json:"descriptionDrift"`
	Orphaned            bool   `json:"orphaned"`
}

// VirtualServer is the API read model.
type VirtualServer struct {
	Slug         string              `json:"slug"`
	Name         string              `json:"name"`
	Description  string              `json:"description"`
	Enabled      bool                `json:"enabled"`
	AuthRequired bool                `json:"authRequired"`
	PII          pii.Config          `json:"pii"`
	Policy       policy.Config       `json:"policy"`
	RateLimit    *int                `json:"rateLimitPerMinute"`
	Endpoint     string              `json:"endpoint"`
	Tools        []ResolvedSelection `json:"tools"`
	Version      int64               `json:"version"`
	CreatedAt    time.Time           `json:"createdAt"`
	UpdatedAt    time.Time           `json:"updatedAt"`
}

// VirtualServerInput is the writable part of a virtual server.
type VirtualServerInput struct {
	Name         string
	Description  string
	Enabled      bool
	AuthRequired bool
	PII          pii.Config
	Policy       policy.Config
	RateLimit    *int
	Tools        []ToolSelection
}

// VirtualServerPatch is a JSON merge patch; a non-nil Tools replaces the list.
type VirtualServerPatch struct {
	Name, Description *string
	Enabled           *bool
	AuthRequired      *bool
	PII               *pii.Config
	Policy            *policy.Config
	RateLimitSet      bool
	RateLimit         *int
	Tools             *[]ToolSelection
}

type vsRow struct {
	VirtualServer
	id string
}

const vsSelect = `
	SELECT v.id::text, v.slug, v.name, v.description, v.enabled, v.auth_required, v.pii_config, v.tool_policy, v.rate_limit_per_minute, v.version, v.created_at, v.updated_at
	FROM virtual_servers v WHERE v.tenant_id = ` + tenantSQL

func scanVS(row scanner) (*vsRow, error) {
	var r vsRow
	var piiRaw, polRaw []byte
	if err := row.Scan(&r.id, &r.Slug, &r.Name, &r.Description, &r.Enabled, &r.AuthRequired, &piiRaw, &polRaw, &r.RateLimit, &r.Version, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(piiRaw, &r.PII); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(polRaw, &r.Policy); err != nil {
		return nil, err
	}
	r.Policy = policy.Normalize(r.Policy)
	r.Endpoint = "/mcp/v1/virtual/" + r.Slug
	r.Tools = []ResolvedSelection{}
	return &r, nil
}

// loadSelections fills Tools of the given rows in operator-chosen order.
func (t *Tx) loadSelections(ctx context.Context, vs []*vsRow) error {
	if len(vs) == 0 {
		return nil
	}
	byID := map[string]*vsRow{}
	ids := make([]string, 0, len(vs))
	for _, v := range vs {
		byID[v.id] = v
		ids = append(ids, v.id)
	}
	rows, err := t.q.Query(ctx, `
		SELECT vt.virtual_server_id::text, u.slug, vt.tool_name, COALESCE(vt.alias, ''), COALESCE(vt.description, ''), COALESCE(ut.description,''),
		       vt.description IS NOT NULL AND vt.description_basis IS DISTINCT FROM ut.description, ut.name IS NULL
		FROM virtual_server_tools vt JOIN upstreams u ON u.id = vt.upstream_id
		LEFT JOIN upstream_tools ut ON ut.upstream_id = vt.upstream_id AND ut.name = vt.tool_name
		WHERE vt.virtual_server_id = ANY($1::uuid[])
		ORDER BY vt.virtual_server_id, vt.position`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var s ToolSelection
		var upDesc string
		var drift, orphan bool
		if err := rows.Scan(&id, &s.Upstream, &s.Tool, &s.Alias, &s.Description, &upDesc, &drift, &orphan); err != nil {
			return err
		}
		v := byID[id]
		v.Tools = append(v.Tools, ResolvedSelection{ToolSelection: s, ExposedName: config.ExposedName(s.Upstream, s.Tool, s.Alias),
			UpstreamDescription: upDesc, DescriptionDrift: drift, Orphaned: orphan})
	}
	return rows.Err()
}

func (t *Tx) getVS(ctx context.Context, slug string, lock bool) (*vsRow, error) {
	q := vsSelect + ` AND v.slug = $1`
	if lock {
		q += ` FOR UPDATE OF v`
	}
	r, err := scanVS(t.q.QueryRow(ctx, q, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierror.NotFound("virtual server", slug)
	}
	if err != nil {
		return nil, err
	}
	if err := t.loadSelections(ctx, []*vsRow{r}); err != nil {
		return nil, err
	}
	return r, nil
}

// VirtualServerVersion returns the ETag version of a virtual server.
func (t *Tx) VirtualServerVersion(ctx context.Context, slug string) (int64, error) {
	r, err := t.getVS(ctx, slug, false)
	if err != nil {
		return 0, err
	}
	return r.Version, nil
}

// GetVirtualServer returns one virtual server.
func (t *Tx) GetVirtualServer(ctx context.Context, slug string) (*VirtualServer, error) {
	r, err := t.getVS(ctx, slug, false)
	if err != nil {
		return nil, err
	}
	return &r.VirtualServer, nil
}

// ListVirtualServers pages through virtual servers ordered by slug.
func (t *Tx) ListVirtualServers(ctx context.Context, after string, limit int, enabled *bool) ([]VirtualServer, bool, error) {
	rows, err := t.q.Query(ctx, vsSelect+`
		AND v.slug > $1 AND ($2::boolean IS NULL OR v.enabled = $2)
		ORDER BY v.slug LIMIT $3`, after, enabled, limit+1)
	if err != nil {
		return nil, false, err
	}
	var list []*vsRow
	for rows.Next() {
		r, err := scanVS(rows)
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
	if err := t.loadSelections(ctx, list); err != nil {
		return nil, false, err
	}
	out := make([]VirtualServer, len(list))
	for i, r := range list {
		out[i] = r.VirtualServer
	}
	return out, more, nil
}

// validateSelections checks every selection against the database and reports all problems.
func (t *Tx) validateSelections(ctx context.Context, sels []ToolSelection) ([]apierror.FieldError, error) {
	var errs []apierror.FieldError
	slugSet := map[string]bool{}
	for _, s := range sels {
		slugSet[s.Upstream] = true
	}
	slugs := make([]string, 0, len(slugSet))
	for s := range slugSet {
		slugs = append(slugs, s)
	}

	known := map[string]bool{}
	rows, err := t.q.Query(ctx, `SELECT slug FROM upstreams WHERE tenant_id = `+tenantSQL+` AND slug = ANY($1)`, slugs)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return nil, err
		}
		known[s] = true
	}
	rows.Close()

	tools := map[string]bool{}
	rows, err = t.q.Query(ctx, `
		SELECT u.slug, ut.name FROM upstream_tools ut JOIN upstreams u ON u.id = ut.upstream_id
		WHERE u.tenant_id = `+tenantSQL+` AND u.slug = ANY($1)`, slugs)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var s, n string
		if err := rows.Scan(&s, &n); err != nil {
			rows.Close()
			return nil, err
		}
		tools[s+"\x00"+n] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	exposed := map[string]int{}
	for i, s := range sels {
		p := fmt.Sprintf("/tools/%d", i)
		switch {
		case !ValidSlug(s.Upstream):
			continue // already reported by input validation
		case !known[s.Upstream]:
			errs = append(errs, fe(p+"/upstream", apierror.CodeUnknownUpstream, fmt.Sprintf("upstream %q does not exist", s.Upstream)))
			continue
		case !tools[s.Upstream+"\x00"+s.Tool]:
			errs = append(errs, fe(p+"/tool", apierror.CodeUnknownTool,
				fmt.Sprintf("upstream %q has no cached tool %q; call refresh-tools if it was added recently", s.Upstream, s.Tool)))
			continue
		}
		name := config.ExposedName(s.Upstream, s.Tool, s.Alias)
		if first, dup := exposed[name]; dup {
			errs = append(errs, fe(p, apierror.CodeDuplicateExposed,
				fmt.Sprintf("exposed name %q is already used by /tools/%d; set an alias to disambiguate", name, first)))
			continue
		}
		exposed[name] = i
	}
	return errs, nil
}

func sameSelections(cur []ResolvedSelection, want []ToolSelection) bool {
	if len(cur) != len(want) {
		return false
	}
	for i := range cur {
		// A drifted override is rewritten so that re-saving acknowledges the upstream change.
		if cur[i].ToolSelection != want[i] || cur[i].DescriptionDrift {
			return false
		}
	}
	return true
}

func (t *Tx) writeSelections(ctx context.Context, vsID string, sels []ToolSelection) error {
	if _, err := t.q.Exec(ctx, `DELETE FROM virtual_server_tools WHERE virtual_server_id = $1::uuid`, vsID); err != nil {
		return err
	}
	for i, s := range sels {
		if _, err := t.q.Exec(ctx, `
			INSERT INTO virtual_server_tools (virtual_server_id, upstream_id, tool_name, alias, position, description, description_basis)
			SELECT $1::uuid, u.id, $3, NULLIF($4, ''), $5, NULLIF($6, ''),
			       CASE WHEN $6 <> '' THEN (SELECT ut.description FROM upstream_tools ut WHERE ut.upstream_id = u.id AND ut.name = $3) END
			FROM upstreams u
			WHERE u.tenant_id = `+tenantSQL+` AND u.slug = $2`, vsID, s.Upstream, s.Tool, s.Alias, i, s.Description); err != nil {
			return err
		}
	}
	return nil
}

// PutVirtualServer creates or replaces a virtual server and its tool list atomically.
func (t *Tx) PutVirtualServer(ctx context.Context, slug string, in VirtualServerInput, ifMatch *int64, createOnly bool) (*VirtualServer, Outcome, error) {
	t.auditTarget("virtual-server", slug)
	if in.Tools == nil {
		in.Tools = []ToolSelection{}
	}
	in.PII = normalizePII(in.PII)
	in.Policy = policy.Normalize(in.Policy)
	errs := append(SlugErrors("/slug", slug), ValidateVirtualServerInput(in)...)
	selErrs, err := t.validateSelections(ctx, in.Tools)
	if err != nil {
		return nil, "", err
	}
	if errs = append(errs, selErrs...); len(errs) > 0 {
		return nil, "", apierror.Validation(errs)
	}

	piiJSON, err := json.Marshal(in.PII)
	if err != nil {
		return nil, "", err
	}
	polJSON, err := json.Marshal(in.Policy)
	if err != nil {
		return nil, "", err
	}
	cur, err := t.getVS(ctx, slug, true)
	var nf *apierror.Error
	if errors.As(err, &nf) && nf.Code == apierror.CodeNotFound {
		if ifMatch != nil {
			return nil, "", apierror.PreconditionFailed()
		}
		var id string
		if err := t.q.QueryRow(ctx, `
			INSERT INTO virtual_servers (tenant_id, slug, name, description, enabled, auth_required, pii_config, tool_policy, rate_limit_per_minute)
			VALUES (`+tenantSQL+`, $1, $2, $3, $4, $5, $6, $7, $8) RETURNING id::text`,
			slug, in.Name, in.Description, in.Enabled, in.AuthRequired, piiJSON, polJSON, in.RateLimit).Scan(&id); err != nil {
			return nil, "", mapWriteErr(err, "virtual server", slug)
		}
		if err := t.writeSelections(ctx, id, in.Tools); err != nil {
			return nil, "", err
		}
		v, err := t.GetVirtualServer(ctx, slug)
		return v, Created, err
	}
	if err != nil {
		return nil, "", err
	}
	if createOnly {
		return nil, "", apierror.AlreadyExists("virtual server", slug)
	}
	if ifMatch != nil && *ifMatch != cur.Version {
		return nil, "", apierror.PreconditionFailed()
	}
	if in.Name == cur.Name && in.Description == cur.Description && in.Enabled == cur.Enabled && in.AuthRequired == cur.AuthRequired && reflect.DeepEqual(in.PII, cur.PII) && reflect.DeepEqual(in.Policy, cur.Policy) && equalInt(in.RateLimit, cur.RateLimit) && sameSelections(cur.Tools, in.Tools) {
		return &cur.VirtualServer, Unchanged, nil
	}
	if _, err := t.q.Exec(ctx, `
		UPDATE virtual_servers SET name=$2, description=$3, enabled=$4, auth_required=$5, pii_config=$6, tool_policy=$7, rate_limit_per_minute=$8, version = version + 1, updated_at = now()
		WHERE id = $1::uuid`, cur.id, in.Name, in.Description, in.Enabled, in.AuthRequired, piiJSON, polJSON, in.RateLimit); err != nil {
		return nil, "", err
	}
	if !sameSelections(cur.Tools, in.Tools) {
		if err := t.writeSelections(ctx, cur.id, in.Tools); err != nil {
			return nil, "", err
		}
	}
	v, err := t.GetVirtualServer(ctx, slug)
	return v, Updated, err
}

// PatchVirtualServer applies a JSON merge patch.
func (t *Tx) PatchVirtualServer(ctx context.Context, slug string, p VirtualServerPatch, ifMatch *int64) (*VirtualServer, error) {
	t.auditTarget("virtual-server", slug)
	cur, err := t.getVS(ctx, slug, true)
	if err != nil {
		return nil, err
	}
	in := VirtualServerInput{Name: cur.Name, Description: cur.Description, Enabled: cur.Enabled, AuthRequired: cur.AuthRequired, PII: cur.PII, Policy: cur.Policy, RateLimit: cur.RateLimit}
	if p.RateLimitSet {
		in.RateLimit = p.RateLimit
	}
	for _, r := range cur.Tools {
		in.Tools = append(in.Tools, r.ToolSelection)
	}
	if p.Name != nil {
		in.Name = *p.Name
	}
	if p.Description != nil {
		in.Description = *p.Description
	}
	if p.Enabled != nil {
		in.Enabled = *p.Enabled
	}
	if p.AuthRequired != nil {
		in.AuthRequired = *p.AuthRequired
	}
	if p.PII != nil {
		in.PII = *p.PII
	}
	if p.Policy != nil {
		in.Policy = *p.Policy
	}
	if p.Tools != nil {
		in.Tools = slices.Clone(*p.Tools)
	}
	v, _, err := t.PutVirtualServer(ctx, slug, in, ifMatch, false)
	return v, err
}

// DeleteVirtualServer removes a virtual server and its selections.
func (t *Tx) DeleteVirtualServer(ctx context.Context, slug string, ifMatch *int64) error {
	t.auditTarget("virtual-server", slug)
	cur, err := t.getVS(ctx, slug, true)
	if err != nil {
		return err
	}
	if ifMatch != nil && *ifMatch != cur.Version {
		return apierror.PreconditionFailed()
	}
	_, err = t.q.Exec(ctx, `DELETE FROM virtual_servers WHERE id = $1::uuid`, cur.id)
	return err
}

// normalizePII makes equivalent configs compare equal (nil vs empty slices).
func normalizePII(c pii.Config) pii.Config {
	if len(c.Detectors) == 0 {
		c.Detectors = nil
	}
	if len(c.CustomRules) == 0 {
		c.CustomRules = nil
	}
	return c
}
