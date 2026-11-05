package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

// SessionStore is authoritative on every authenticated request; errors deny access.
type SessionStore interface {
	Save(context.Context, Session) error
	Valid(context.Context, string) (bool, error)
	Revoke(context.Context, string) error
	RevokeSubject(context.Context, string) error
}
type PostgresSessions struct {
	Pool   *pgxpool.Pool
	Box    *secrets.Box
	Issuer string
}

func (s PostgresSessions) Save(ctx context.Context, session Session) error {
	if s.Box != nil {
		if session.Version != 1 || session.Issuer == "" || session.Issuer != s.Issuer || session.ID == "" {
			return errors.New("invalid server session")
		}
		raw, err := json.Marshal(sessionRecord{Session: session, Claims: session.Claims, ClaimsExpire: session.ClaimsExpire})
		if err != nil {
			return err
		}
		cipher, err := s.Box.BoundOnly().SealBound(raw, session.ID, "oidc.browser-session")
		if err != nil {
			return err
		}
		_, err = s.Pool.Exec(ctx, `INSERT INTO browser_sessions(id,subject,issuer,expires_at,model_version,session_ciphertext) VALUES($1,$2,$3,$4,1,$5)`, session.ID, session.Subject, session.Issuer, session.Expires, cipher)
		return err
	}
	if session.Version != 0 {
		return errors.New("server session requires encrypted storage")
	}
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
	_, err := s.Pool.Exec(ctx, `UPDATE browser_sessions SET revoked_at=now() WHERE subject=$1 AND revoked_at IS NULL AND ($2='' OR issuer=$2)`, subject, s.Issuer)
	return err
}

func (s PostgresSessions) SecureSessions() bool { return s.Box != nil && s.Issuer != "" }
func (s PostgresSessions) LoadSession(ctx context.Context, id, issuer string) (Session, bool, error) {
	if !s.SecureSessions() || issuer != s.Issuer {
		return Session{}, false, errors.New("secure session store unavailable")
	}
	var raw []byte
	var subject string
	var expires time.Time
	err := s.Pool.QueryRow(ctx, `SELECT session_ciphertext,subject,expires_at FROM browser_sessions WHERE id=$1 AND issuer=$2 AND model_version=1 AND revoked_at IS NULL AND expires_at>now()`, id, issuer).Scan(&raw, &subject, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, err
	}
	plain, err := s.Box.BoundOnly().OpenBound(raw, id, "oidc.browser-session")
	if err != nil {
		return Session{}, false, errors.New("session integrity check failed")
	}
	var record sessionRecord
	if json.Unmarshal(plain, &record) != nil {
		return Session{}, false, errors.New("invalid session record")
	}
	value := record.Session
	// pg timestamptz rounds nanoseconds. Identity and expiration remain bound to
	// the encrypted record and the separately indexed revocation row.
	if value.ID != id || value.Issuer != issuer || value.Subject != subject || value.Version != 1 || !value.Expires.Truncate(time.Microsecond).Equal(expires) {
		return Session{}, false, errors.New("session metadata mismatch")
	}
	value.Claims = record.Claims
	value.ClaimsExpire = record.ClaimsExpire
	return value, true, nil
}

// RevokeIssuerSubject is unambiguous even when issuer is empty (legacy rows).
func (s PostgresSessions) RevokeIssuerSubject(ctx context.Context, issuer, subject string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE browser_sessions SET revoked_at=now() WHERE issuer=$1 AND subject=$2 AND revoked_at IS NULL`, issuer, subject)
	return err
}
