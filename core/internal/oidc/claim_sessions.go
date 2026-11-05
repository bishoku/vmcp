package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/bishoku/vmcp/core/internal/agentauth"
)

// ServerSessions is the secure extension of the legacy revocation interface.
// Claims never enter cookies or cookie-only/legacy session stores.
type ServerSessions interface {
	SessionStore
	SecureSessions() bool
	LoadSession(context.Context, string, string) (Session, bool, error)
}

func (p *Provider) serverSessions() (ServerSessions, bool) {
	s, ok := p.cfg.Sessions.(ServerSessions)
	return s, ok && s.SecureSessions()
}
func (p *Provider) ClaimSessionsEnabled() bool {
	_, ok := p.serverSessions()
	return ok && p.cfg.ClaimPaths != nil
}

// captureClaims keeps only currently requested string or string-array values.
// Flattening path names preserves the matcher's exact-key precedence while
// preventing an object path from capturing unrelated sibling claims or tokens.
func captureClaims(claims map[string]any, paths []string) (map[string]any, error) {
	if len(paths) > 256 {
		return nil, errors.New("too many session claim paths")
	}
	out := map[string]any{}
	for _, path := range paths {
		if path == "" || len(path) > 512 {
			continue
		}
		switch value := agentauth.Lookup(claims, path).(type) {
		case string:
			if len(value) <= 8192 {
				out[path] = value
			}
		case []any:
			if len(value) > 256 {
				continue
			}
			items := []string{}
			valid := true
			for _, v := range value {
				if s, ok := v.(string); ok {
					if len(s) > 8192 {
						valid = false
						break
					}
					items = append(items, s)
				}
			}
			if valid {
				out[path] = items
			}
		case []string:
			if len(value) > 256 {
				continue
			}
			valid := true
			for _, v := range value {
				if len(v) > 8192 {
					valid = false
					break
				}
			}
			if valid {
				out[path] = append([]string(nil), value...)
			}
		}
	}
	raw, err := json.Marshal(out)
	if err != nil || len(raw) > 65536 {
		return nil, errors.New("session claim subset exceeds limit")
	}
	return out, nil
}

type sessionReference struct {
	ID      string    `json:"sid"`
	Version int       `json:"v"`
	Expires time.Time `json:"exp"`
}

// record is encrypted in its entirety. Tokens obtained from the OAuth exchange
// are never fields of this structure; Session.Claims is excluded from cookies.
type sessionRecord struct {
	Session
	Claims       map[string]any `json:"claims"`
	ClaimsExpire time.Time      `json:"claimsExp"`
}

func (p *Provider) sessionCookieName() string {
	if _, ok := p.serverSessions(); ok {
		return "vmcp_session_v2"
	}
	return sessionCookie
}
