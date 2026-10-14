package virtual

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Record is the part of a session that survives the node that opened it.
type Record struct {
	Owner              Owner
	ID                 string
	Slug               string
	KeySlug            string
	Version            string
	LastSeen           time.Time
	Initialized        bool
	ClientCapabilities json.RawMessage
	LoggingLevel       string
}

// SessionStore persists session records so any data-plane node can resume a session.
type SessionStore interface {
	Save(ctx context.Context, r Record) error
	MarkInitialized(ctx context.Context, id string) error
	Load(ctx context.Context, id string) (Record, bool, error)
	Delete(ctx context.Context, id string) error
	Touch(ctx context.Context, id string) error
	// Purge removes records idle for longer than idle.
	Purge(ctx context.Context, idle time.Duration) (int64, error)
}

// PGSessionStore keeps session records in PostgreSQL.
type PGSessionStore struct{ Pool *pgxpool.Pool }

func (s PGSessionStore) Save(ctx context.Context, r Record) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO sessions (id, virtual_server, key_slug, protocol_version, client_capabilities, owner_mode, credential_id, owner_issuer, owner_subject)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT (id) DO NOTHING`, r.ID, r.Slug, r.KeySlug, r.Version, r.ClientCapabilities, r.Owner.Mode, r.Owner.CredentialID, r.Owner.Issuer, r.Owner.Subject)
	return err
}

func (s PGSessionStore) Load(ctx context.Context, id string) (Record, bool, error) {
	r := Record{ID: id}
	err := s.Pool.QueryRow(ctx, `
		SELECT virtual_server, key_slug, protocol_version, last_seen_at, initialized, client_capabilities, owner_mode, credential_id, owner_issuer, owner_subject, COALESCE(logging_level, 'warning') FROM sessions WHERE id = $1 AND terminated_at IS NULL`, id).
		Scan(&r.Slug, &r.KeySlug, &r.Version, &r.LastSeen, &r.Initialized, &r.ClientCapabilities, &r.Owner.Mode, &r.Owner.CredentialID, &r.Owner.Issuer, &r.Owner.Subject, &r.LoggingLevel)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, false, nil
	}
	r.Owner.Principal = r.KeySlug
	return r, err == nil, err
}

func (s PGSessionStore) Delete(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE sessions SET terminated_at = COALESCE(terminated_at, now()) WHERE id = $1`, id)
	return err
}

func (s PGSessionStore) Touch(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE sessions SET last_seen_at = now() WHERE id = $1 AND terminated_at IS NULL`, id)
	return err
}

func (s PGSessionStore) Purge(ctx context.Context, idle time.Duration) (int64, error) {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE last_seen_at < now() - make_interval(secs => $1) OR terminated_at < now() - make_interval(secs => $1)`, idle.Seconds())
	return tag.RowsAffected(), err
}

func (s PGSessionStore) MarkInitialized(ctx context.Context, id string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE sessions SET initialized = true WHERE id = $1 AND terminated_at IS NULL`, id)
	if err == nil && tag.RowsAffected() != 1 {
		return errors.New("session no longer exists")
	}
	return err
}
