package agentauth

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TokenRevocations checks exact-token SHA-256 digests, never raw tokens.
type TokenRevocations interface {
	Revoked(context.Context, []byte) (bool, error)
}
type PostgresRevocations struct{ Pool *pgxpool.Pool }

func (s PostgresRevocations) Revoked(ctx context.Context, hash []byte) (bool, error) {
	var revoked bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM revoked_jwts WHERE token_hash=$1 AND expires_at>now())`, hash).Scan(&revoked)
	return revoked, err
}

func (s PostgresRevocations) Revoke(ctx context.Context, hash []byte, expiresAt time.Time) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO revoked_jwts(token_hash,expires_at) VALUES($1,$2) ON CONFLICT(token_hash) DO UPDATE SET expires_at=GREATEST(revoked_jwts.expires_at,EXCLUDED.expires_at)`, hash, expiresAt)
	return err
}

