package store

import (
	"context"
	"testing"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/authz"
	"github.com/bishoku/vmcp/core/internal/testdb"
)

func TestAuditSQLVisibilityMatchesCentralPolicy(t *testing.T) {
	ctx := context.Background()
	url := testdb.New(t)
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	s := Store{Pool: pool}
	const owner = "11111111-1111-1111-1111-111111111111"
	const other = "22222222-2222-2222-2222-222222222222"
	const project = "33333333-3333-3333-3333-333333333333"
	type fixture struct {
		id     string
		access authz.AuditPayload
	}
	var fixtures []fixture
	resource := func(kind, team, pid string) audit.AccessDomain {
		t.Helper()
		id, _ := audit.NewID()
		var epoch string
		if err := pool.QueryRow(ctx, `INSERT INTO audit_resource_epochs(resource_kind,resource_id,owner_team_id,project_id,start_revision) VALUES($1,$2,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid,1) RETURNING epoch_id::text`, kind, id, team, pid).Scan(&epoch); err != nil {
			t.Fatal(err)
		}
		return audit.AccessDomain{Kind: "target", ResourceKind: kind, ResourceID: id, OwnerTeamID: team, ProjectID: pid, EpochID: epoch}
	}
	a := resource("virtual_server", owner, project)
	b := resource("virtual_server", other, project)
	up := resource("upstream", owner, "")
	unmanaged := resource("upstream", "", "")
	for _, domains := range [][]audit.AccessDomain{{a}, {b}, {up}, {unmanaged}, {a, b}, nil} {
		id, _ := audit.NewID()
		c := &audit.AccessContext{Tenant: "default", Classification: "resource", Origin: "test", Revision: 1, Domains: domains}
		if len(domains) == 0 {
			c.Classification = "tenant"
		}
		if err := audit.Persist(ctx, pool, audit.Event{EventID: id, Status: "ok", Access: c}); err != nil {
			t.Fatal(err)
		}
		access, err := s.AuditPayloadAccess(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		fixtures = append(fixtures, fixture{id, access})
	}
	legacyID, _ := audit.NewID()
	if err := audit.Persist(ctx, pool, audit.Event{EventID: legacyID, Status: "ok"}); err != nil {
		t.Fatal(err)
	}
	legacy, err := s.AuditPayloadAccess(ctx, legacyID)
	if err != nil {
		t.Fatal(err)
	}
	fixtures = append(fixtures, fixture{legacyID, legacy})
	who := authz.Principal{Kind: "oidc", Issuer: "https://idp", ID: "reader"}
	for _, scope := range []authz.Scope{{Kind: "tenant"}, {Kind: "team", ID: owner}, {Kind: "project", ID: project, OwnerTeam: owner}, {Kind: "project", ID: project, OwnerTeam: other}, {Kind: "virtual_server", ID: a.ResourceID, Epoch: a.EpochID}, {Kind: "virtual_server", ID: a.ResourceID}, {Kind: "unmanaged"}} {
		t.Run(scope.Kind+scope.ID+scope.OwnerTeam+scope.Epoch, func(t *testing.T) {
			p := authz.Policy{Tenant: "default", Revision: 1, Roles: []authz.Role{{ID: "role", Permissions: []string{"audit.events.read", "audit.legacy.events.read"}}}, Assignments: []authz.Assignment{{ID: "assignment", RoleID: "role", Principal: who, Scope: scope, Enabled: true}}}
			rows, _, err := s.Reader().ListAudit(WithAuditPolicy(ctx, p, who), AuditFilter{}, 200)
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]bool{}
			for _, row := range rows {
				got[row.EventID] = true
			}
			for _, f := range fixtures {
				want := p.CanReadAuditPermission(who, f.access, "audit.events.read")
				if got[f.id] != want {
					t.Fatalf("SQL and central policy disagree: %+v want %v", f.access, want)
				}
			}
		})
	}
}
