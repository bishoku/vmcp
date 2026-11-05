package authz

import (
	"fmt"
	"testing"
)

// Isolates policy evaluation cost, not HTTP, SQL, OIDC, or payload decryption.
// Most grants belong to other subjects, as in a multi-team control plane.
func BenchmarkPolicyScaling(b *testing.B) {
	for _, count := range []int{10, 100, 1000, 10000} {
		b.Run(fmt.Sprintf("assignments=%d", count), func(b *testing.B) {
			who := Principal{Kind: "control_token", ID: "reader"}
			p := Policy{Tenant: "default", Revision: 1, Roles: []Role{{ID: "reader", Permissions: []string{"upstreams.read", "audit.events.read", "audit.identities.read", "audit.details.read", "audit.payloads.request.read"}}}}
			for i := 0; i < count; i++ {
				principal := Principal{Kind: "control_token", ID: fmt.Sprintf("subject-%d", i)}
				if i == count-1 {
					principal = who
				}
				p.Assignments = append(p.Assignments, Assignment{ID: fmt.Sprintf("grant-%d", i), RoleID: "reader", Principal: principal, Scope: Scope{Kind: "team", ID: "team"}, CaptureScopes: []string{"virtual"}, Enabled: true})
			}
			resource := Resource{Tenant: "default", Kind: "virtual_server", ID: "server", OwnerTeam: "team", Epoch: "epoch"}
			event := AuditPayload{Tenant: "default", Classification: "resource", Direction: "request", CaptureScope: "virtual", Domains: []Resource{resource}}
			if err := p.Validate(); err != nil {
				b.Fatal(err)
			}
			b.Run("decision", func(b *testing.B) {
				b.ReportAllocs()
				q := Request{Principal: who, Permission: "audit.events.read", Resource: resource}
				for b.Loop() {
					if !p.Decide(q).Allowed {
						b.Fatal("unexpected denial")
					}
				}
			})
			b.Run("audit-page-50", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					for i := 0; i < 50; i++ {
						for _, permission := range []string{"audit.events.read", "audit.identities.read", "audit.details.read"} {
							if !p.CanReadAuditPermission(who, event, permission) {
								b.Fatal("unexpected metadata denial")
							}
						}
						if !p.CanReadPayload(who, event) {
							b.Fatal("unexpected payload denial")
						}
					}
				}
			})
		})
	}
}
