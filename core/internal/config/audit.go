package config

import "github.com/bishoku/vmcp/core/internal/audit"

// ConsumerAudit builds an operation-owned copy from this authorized snapshot.
// Hand-built/pre-upgrade snapshots without provenance remain legacy.
func (s *Snapshot) ConsumerAudit(v VirtualServer) *audit.AccessContext {
	if s == nil || s.AuditTenant == "" || v.AuditDomain.EpochID == "" {
		return nil
	}
	return &audit.AccessContext{Tenant: s.AuditTenant, Classification: "resource", Origin: "virtual", Revision: s.Revision, Domains: []audit.AccessDomain{v.AuditDomain}}
}
func (s *Snapshot) TenantAudit(origin string) *audit.AccessContext {
	if s == nil || s.AuditTenant == "" {
		return nil
	}
	return &audit.AccessContext{Tenant: s.AuditTenant, Classification: "tenant", Origin: origin, Revision: s.Revision}
}
