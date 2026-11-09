package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/bishoku/vmcp/core/internal/admission"
	"github.com/jackc/pgx/v5/pgxpool"
)

func LoadAdmissionSettings(ctx context.Context, pool *pgxpool.Pool) (admission.Settings, error) {
	return loadAdmissionSettings(ctx, pool)
}
func loadAdmissionSettings(ctx context.Context, q dbtx) (admission.Settings, error) {
	s := admission.DefaultSettings()
	var raw []byte
	var version int64
	if err := q.QueryRow(ctx, `SELECT settings,version FROM admission_settings WHERE singleton`).Scan(&raw, &version); err != nil {
		return s, err
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return s, err
	}
	s.Revision = version
	return s, s.Validate()
}

var ErrAdmissionSettingsConflict = errors.New("admission settings revision conflict")

func (t *Tx) SaveAdmissionSettings(ctx context.Context, s admission.Settings, version int64) error {
	if err := s.Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tag, err := t.q.Exec(ctx, `UPDATE admission_settings SET settings=$1,version=version+1 WHERE singleton AND version=$2`, raw, version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrAdmissionSettingsConflict
	}
	t.targets["admission-settings"] = true
	return nil
}

func (t *Tx) AdmissionSettings(ctx context.Context) (admission.Settings, error) {
	return loadAdmissionSettings(ctx, t.q)
}
