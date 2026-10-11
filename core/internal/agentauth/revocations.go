package agentauth

import (
	"context"
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
