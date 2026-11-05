package store

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/bishoku/vmcp/core/internal/authz"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LoadOIDCClaimPaths runs only for a verified sign-in. The provider issuer is a
// server setting, never a request-selected trust root. Disabled mappings do not
// expand the captured claim subset; newly required claims need a fresh sign-in.
func LoadOIDCClaimPaths(ctx context.Context, pool *pgxpool.Pool, issuer string) ([]string, error) {
	rows, err := pool.Query(ctx, `SELECT conditions FROM authorization_mappings WHERE enabled AND issuer=$1 AND principal_kind='oidc' ORDER BY id`, issuer)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	paths := []string{}
	seen := map[string]bool{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var conditions []authz.Condition
		if err := json.Unmarshal(raw, &conditions); err != nil {
			return nil, err
		}
		for _, c := range conditions {
			if !seen[c.Path] {
				seen[c.Path] = true
				paths = append(paths, c.Path)
				if len(paths) > 256 {
					return nil, fmt.Errorf("too many session claim paths")
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.Sort(paths)
	return paths, nil
}

// CanOIDCLogin requires an applicable assignment for the verified principal.
func CanOIDCLogin(ctx context.Context, pool *pgxpool.Pool, issuer, subject string, claims map[string]any) (bool, error) {
	p, err := LoadAuthorization(ctx, pool)
	if err != nil {
		return false, err
	}
	who := authz.Principal{Kind: "oidc", Issuer: issuer, ID: subject}
	p = p.ResolveMappings(who, claims)

	for _, a := range p.Assignments {
		if !a.Enabled || a.MappingID != "" || a.Principal != who {
			continue
		}
		for _, r := range p.Roles {
			if r.ID != a.RoleID {
				continue
			}
			for _, permission := range r.Permissions {
				spec, _ := authz.Lookup(permission)
				if !slices.Contains(spec.Scopes, a.Scope.Kind) {
					continue
				}
				if permission == "audit.events.read" {
					return true, nil
				}
				if configurationLoginPermission(permission) {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func configurationLoginPermission(permission string) bool {
	if strings.HasPrefix(permission, "access.") {
		return false
	}
	if strings.HasPrefix(permission, "audit.") {
		return permission == "audit.status.read" || permission == "audit.settings.read" || permission == "audit.redaction.preview" || strings.HasSuffix(permission, ".update")
	}
	return true
}
