package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/jackc/pgx/v5/pgxpool"
)

func LoadAuditSettings(ctx context.Context, pool *pgxpool.Pool) (audit.Settings, error) {
	return loadAuditSettings(ctx, pool)
}
func loadAuditSettings(ctx context.Context, q dbtx) (audit.Settings, error) {
	s := audit.DefaultSettings()
	var raw []byte
	var version int64
	if err := q.QueryRow(ctx, `SELECT settings,version FROM audit_settings WHERE singleton`).Scan(&raw, &version); err != nil {
		return s, err
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return s, err
	}
	s = s.Normalize()
	s.Revision = version
	return s, s.Validate()
}

var ErrAuditSettingsConflict = errors.New("audit settings revision conflict")

func (t *Tx) SaveAuditSettings(ctx context.Context, s audit.Settings, version int64) error {
	if err := s.Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tag, err := t.q.Exec(ctx, `UPDATE audit_settings SET settings=$1,version=version+1 WHERE singleton AND version=$2`, raw, version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrAuditSettingsConflict
	}
	if _, err := t.q.Exec(ctx, `UPDATE audit_export_state SET include_payload=COALESCE(($1::jsonb->'exportPayloads'->>consumer)::boolean,false)`, raw); err != nil {
		return err
	}
	t.targets["audit-settings"] = true
	return nil
}

func (t *Tx) AuditSettings(ctx context.Context) (audit.Settings, error) {
	return loadAuditSettings(ctx, t.q)
}

// Avoid repeatedly decoding and compiling every policy for unrelated API calls.
func LoadAuditSettingsIfChanged(ctx context.Context, pool *pgxpool.Pool, previous audit.Settings) (audit.Settings, error) {
	var version int64
	if err := pool.QueryRow(ctx, `SELECT version FROM audit_settings WHERE singleton`).Scan(&version); err != nil {
		return previous, err
	}
	if version == previous.Revision {
		return previous, nil
	}
	return LoadAuditSettings(ctx, pool)
}
