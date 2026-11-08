package store

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/callercontext"
	"github.com/bishoku/vmcp/core/internal/invocation"
)

type UpstreamInvocationSecurity struct {
	UpstreamID          string                    `json:"upstreamId"`
	Revision            int64                     `json:"revision"`
	Policy              invocation.UpstreamPolicy `json:"policy"`
	SharedServices      []string                  `json:"sharedServices"`
	Tools               []string                  `json:"tools"`
	CallerContextFields []string                  `json:"callerContextFields"`
}

func (t *Tx) invocationTarget(ctx context.Context, slug string) (id string, revision int64, caller *callercontext.Config, err error) {
	err = t.q.QueryRow(ctx, `SELECT id::text,invocation_security_revision,caller_context FROM upstreams WHERE slug=$1 AND tenant_id=`+tenantSQL, slug).Scan(&id, &revision, &caller)
	if err == pgx.ErrNoRows {
		err = apierror.NotFound("upstream", slug)
	}
	return
}

func (t *Tx) UpstreamInvocationSecurity(ctx context.Context, slug string) (UpstreamInvocationSecurity, error) {
	id, revision, caller, err := t.invocationTarget(ctx, slug)
	if err != nil {
		return UpstreamInvocationSecurity{}, err
	}
	settings, err := t.InvocationSecurity(ctx)
	if err != nil {
		return UpstreamInvocationSecurity{}, err
	}
	rows, err := t.q.Query(ctx, `SELECT name FROM upstream_tools WHERE upstream_id=$1::uuid ORDER BY name`, id)
	if err != nil {
		return UpstreamInvocationSecurity{}, err
	}
	tools, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return UpstreamInvocationSecurity{}, err
	}
	fields := []string{}
	if caller != nil && caller.Enabled {
		for _, field := range caller.Fields {
			fields = append(fields, field.Name)
		}
	}
	policy := settings.Upstreams[id]
	policy.HideCredentials()
	return UpstreamInvocationSecurity{
		UpstreamID: id, Revision: revision, Policy: policy, SharedServices: append([]string{}, slices.Sorted(maps.Keys(settings.Services))...),
		Tools: append([]string{}, tools...), CallerContextFields: fields,
	}, nil
}

func (t *Tx) SaveUpstreamInvocationSecurity(ctx context.Context, slug, expectedID string, revision int64, policy invocation.UpstreamPolicy) error {
	id, currentRevision, _, err := t.invocationTarget(ctx, slug)
	if err != nil {
		return err
	}
	if id != expectedID || currentRevision != revision {
		return apierror.PreconditionFailed()
	}
	settings, err := t.InvocationSecurity(ctx)
	if err != nil {
		return err
	}
	previous := settings.Upstreams[id]
	var changedTools []string
	for name, rule := range policy.Tools {
		if old, exists := previous.Tools[name]; !exists || !reflect.DeepEqual(rule, old) {
			changedTools = append(changedTools, name)
		}
	}
	if len(changedTools) > 0 {
		var missing string
		err := t.q.QueryRow(ctx, `SELECT candidate FROM unnest($2::text[]) AS candidate
WHERE NOT EXISTS (SELECT 1 FROM upstream_tools WHERE upstream_id=$1::uuid AND name=candidate)
ORDER BY candidate LIMIT 1`, id, changedTools).Scan(&missing)
		switch err {
		case nil:
			return apierror.New(422, "unknown_invocation_tool", "Unknown upstream tool", fmt.Sprintf("Tool %q is not in the upstream catalog", missing))
		case pgx.ErrNoRows:
		default:
			return err
		}
	}
	if policy.Empty() {
		delete(settings.Upstreams, id)
	} else {
		settings.Upstreams[id] = policy
	}
	t.auditTarget("upstream", slug)
	return t.SaveInvocationSecurity(ctx, settings)
}
