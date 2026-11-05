package authz

// AuditPayload is trusted persisted event context, never a client's scope query.
// Every domain must be authorized (normally the consumer domain; provider-only
// ownership is not a substitute). Tenant-wide mixed diagnostics have no domains.
type AuditPayload struct {
	Tenant            string
	Classification    string
	Direction         string
	CaptureScope      string
	Domains           []Resource
	DeniedDiagnostics bool
}

func (p Policy) CanReadPayload(who Principal, e AuditPayload) bool {
	switch e.Classification {
	case "resource":
		if len(e.Domains) == 0 {
			return false
		}
	case "tenant":
		if len(e.Domains) != 0 {
			return false
		}
	case "legacy":
	default:
		return false
	}
	legacy := e.Classification == "legacy"
	tenant := Resource{Tenant: e.Tenant, Kind: "tenant"}
	allowed := func(permission string, r Resource, audit bool) bool {
		return p.Decide(Request{Principal: who, Permission: permission, Resource: r, Audit: audit, CaptureScope: e.CaptureScope}).Allowed
	}
	if e.DeniedDiagnostics && !allowed("audit.diagnostics.denied.read", tenant, false) {
		return false
	}
	if legacy {
		for _, id := range []string{"audit.events.read", "audit.legacy.events.read"} {
			if !allowed(id, tenant, false) {
				return false
			}
		}
		if !ValidCaptureScope(e.CaptureScope) || (e.Direction != "request" && e.Direction != "response") {
			return allowed("audit.legacy.payloads.unclassified.read", tenant, false)
		}
		if !allowed("audit.legacy.payloads.read", tenant, false) {
			return false
		}
	}
	if e.Direction != "request" && e.Direction != "response" {
		return false
	}
	domains := e.Domains
	if legacy || len(domains) == 0 {
		domains = []Resource{tenant}
	}
	for _, r := range domains {
		if r.Tenant != e.Tenant || !allowed("audit.events.read", r, !legacy) || !allowed("audit.payloads."+e.Direction+".read", r, !legacy) {
			return false
		}
	}
	return true
}

// CanReadAuditPermission applies the same persisted domain rules to metadata and
// sensitive metadata fields. It never uses present-day ownership or parent IDs.
func (p Policy) CanReadAuditPermission(who Principal, e AuditPayload, permission string) bool {
	tenant := Resource{Tenant: e.Tenant, Kind: "tenant"}
	allowed := func(action string, r Resource) bool {
		return p.Decide(Request{Principal: who, Permission: action, Resource: r, Audit: true}).Allowed
	}
	switch e.Classification {
	case "legacy":
		return allowed("audit.events.read", tenant) && allowed("audit.legacy.events.read", tenant) && allowed(permission, tenant)
	case "tenant":
		return len(e.Domains) == 0 && allowed("audit.events.read", tenant) && allowed(permission, tenant)
	case "resource":
		if len(e.Domains) == 0 {
			return false
		}
		for _, d := range e.Domains {
			if d.Tenant != e.Tenant || !allowed("audit.events.read", d) || !allowed(permission, d) {
				return false
			}
		}
		return true
	}
	return false
}
