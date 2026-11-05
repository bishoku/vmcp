package store

import (
	"context"
	"github.com/bishoku/vmcp/core/internal/authz"
	"slices"
	"strings"
)

type auditPolicyKey struct{}
type auditPolicy struct {
	policy authz.Policy
	who    authz.Principal
}

func WithAuditPolicy(ctx context.Context, p authz.Policy, who authz.Principal) context.Context {
	return context.WithValue(ctx, auditPolicyKey{}, auditPolicy{p, who})
}

// SQL predicates are generated from validated policy data using quoted literals.
// They run before pagination and grouping; no payload columns are referenced.
func auditPermissionSQL(p authz.Policy, who authz.Principal, permission string) string {
	return auditPermissionSQLFor(p, who, permission, "audit_events")
}
func auditPermissionSQLFor(p authz.Policy, who authz.Principal, permission, alias string) string {
	if alias != "audit_events" && alias != "o" {
		return "FALSE"
	}

	if p.Validate() != nil {
		return "FALSE"
	}
	spec, ok := authz.Lookup(permission)
	if !ok {
		return "FALSE"
	}
	var domains, tenant []string
	for _, a := range p.Assignments {
		if !a.Enabled || a.MappingID != "" || a.Principal != who || !slices.Contains(spec.Scopes, a.Scope.Kind) {
			continue
		}
		grant := false
		for _, r := range p.Roles {
			if r.ID == a.RoleID && slices.Contains(r.Permissions, permission) {
				grant = true
			}
		}
		if !grant {
			continue
		}
		match := "FALSE"
		switch a.Scope.Kind {
		case "tenant":
			match = "TRUE"
			tenant = append(tenant, "TRUE")
		case "team":
			match = "COALESCE(d.owner_team_id::text,'')=" + literal(a.Scope.ID)
		case "project":
			match = "d.project_id::text=" + literal(a.Scope.ID) + " AND COALESCE(d.owner_team_id::text,'')=" + literal(a.Scope.OwnerTeam)
		case "unmanaged":
			match = "d.owner_team_id IS NULL"
		case "upstream", "virtual_server":
			if a.Scope.Epoch != "" {
				match = "d.resource_kind=" + literal(a.Scope.Kind) + " AND d.resource_id::text=" + literal(a.Scope.ID) + " AND d.epoch_id::text=" + literal(a.Scope.Epoch)
			}
		}
		domains = append(domains, "COALESCE(("+match+"),FALSE)")
	}
	anyDomain := "FALSE"
	if len(domains) > 0 {
		anyDomain = "(" + strings.Join(domains, " OR ") + ")"
	}
	anyTenant := "FALSE"
	if len(tenant) > 0 {
		anyTenant = "TRUE"
	}
	contextMatch := "c.event_id=" + alias + ".event_id AND c.tenant_id=" + alias + ".tenant_id"
	domainWhere := "d.event_id=" + alias + ".event_id AND d.domain_kind IN ('consumer','target')"
	resource := `EXISTS(SELECT 1 FROM audit_access_contexts c WHERE ` + contextMatch + ` AND c.classification='resource') AND EXISTS(SELECT 1 FROM audit_access_domains d WHERE ` + domainWhere + `) AND NOT EXISTS(SELECT 1 FROM audit_access_domains d WHERE ` + domainWhere + ` AND NOT (` + anyDomain + `))`
	tenantCase := `EXISTS(SELECT 1 FROM audit_access_contexts c WHERE ` + contextMatch + ` AND c.classification='tenant') AND NOT EXISTS(SELECT 1 FROM audit_access_domains d WHERE ` + domainWhere + `) AND ` + anyTenant
	legacyGrant := "FALSE"
	if permission == "audit.legacy.events.read" {
		legacyGrant = anyTenant
	} else {
		for _, a := range p.Assignments {
			if !a.Enabled || a.MappingID != "" || a.Principal != who || a.Scope.Kind != "tenant" {
				continue
			}
			for _, r := range p.Roles {
				if r.ID == a.RoleID && slices.Contains(r.Permissions, "audit.legacy.events.read") {
					legacyGrant = "TRUE"
				}
			}
		}
	}
	legacy := `(NOT EXISTS(SELECT 1 FROM audit_access_contexts c WHERE c.event_id=` + alias + `.event_id) OR EXISTS(SELECT 1 FROM audit_access_contexts c WHERE ` + contextMatch + ` AND c.classification='legacy')) AND ` + anyTenant + ` AND ` + legacyGrant
	return "(" + alias + ".tenant_id=" + literal(p.Tenant) + " AND ((" + resource + ") OR (" + tenantCase + ") OR (" + legacy + ")))"
}
