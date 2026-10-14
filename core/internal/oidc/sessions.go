package oidc

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionStore is authoritative on every authenticated request; errors deny access.
type SessionStore interface {
	Save(context.Context, Session) error
	Valid(context.Context, string) (bool, error)
	Revoke(context.Context, string) error
	RevokeSubject(context.Context, string) error
}
type PostgresSessions struct{ Pool *pgxpool.Pool }

func (s PostgresSessions) Save(ctx context.Context, session Session) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO browser_sessions(id,subject,expires_at) VALUES($1,$2,$3)`, session.ID, session.Subject, session.Expires)
	return err
}
func (s PostgresSessions) Valid(ctx context.Context, id string) (bool, error) {
	var valid bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM browser_sessions WHERE id=$1 AND revoked_at IS NULL AND expires_at>now())`, id).Scan(&valid)
	return valid, err
}
func (s PostgresSessions) Revoke(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE browser_sessions SET revoked_at=now() WHERE id=$1 AND revoked_at IS NULL`, id)
	return err
}

// RevokeSubject supports operator account containment across all control-plane nodes.
func (s PostgresSessions) RevokeSubject(ctx context.Context, subject string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE browser_sessions SET revoked_at=now() WHERE subject=$1 AND revoked_at IS NULL`, subject)
	return err
}
