package store

import (
	"context"
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/authz"
	"github.com/jackc/pgx/v5"
)

// AuditPayloadAccess reads only immutable metadata/provenance, never ciphertext.
func (s *Store) AuditPayloadAccess(ctx context.Context, id string) (authz.AuditPayload, error) {
	records, err := s.AuditAccessBatch(ctx, []string{id})
	if err != nil {
		return authz.AuditPayload{}, err
	}
	e, ok := records[id]
	if !ok {
		return authz.AuditPayload{}, pgx.ErrNoRows
	}
	return e, nil
}

// AuditAccessBatch supports event capability decoration without N+1 lookups.
func (s *Store) AuditAccessBatch(ctx context.Context, ids []string) (map[string]authz.AuditPayload, error) {
	return auditAccessBatch(ctx, s.Pool, ids)
}

func auditAccessBatch(ctx context.Context, q dbtx, ids []string) (map[string]authz.AuditPayload, error) {
	out := map[string]authz.AuditPayload{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `SELECT a.event_id::text,a.tenant_id,a.direction,a.scope,COALESCE(c.classification,'legacy'),COALESCE((SELECT jsonb_agg(jsonb_build_object('Tenant',a.tenant_id,'Kind',d.resource_kind,'ID',d.resource_id::text,'Project',COALESCE(d.project_id::text,''),'OwnerTeam',COALESCE(d.owner_team_id::text,''),'Epoch',d.epoch_id::text)) FROM audit_access_domains d WHERE d.event_id=a.event_id AND d.domain_kind IN ('consumer','target')),'[]'::jsonb) FROM audit_events a LEFT JOIN audit_access_contexts c ON c.event_id=a.event_id AND c.tenant_id=a.tenant_id WHERE a.event_id=ANY($1::uuid[]) AND a.tenant_id=vmcp_tenant_id()`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var e authz.AuditPayload
		var raw []byte
		if err = rows.Scan(&id, &e.Tenant, &e.Direction, &e.CaptureScope, &e.Classification, &raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &e.Domains); err != nil {
			return nil, err
		}
		e.DeniedDiagnostics = e.CaptureScope == "omni_diagnostics"
		out[id] = e
	}
	return out, rows.Err()
}
