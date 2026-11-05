package store

import (
	"context"
	"testing"

	"github.com/bishoku/vmcp/core/internal/authz"
	"github.com/bishoku/vmcp/core/internal/testdb"
)

func TestConfigurationSQLMatchesCentralDecisions(t *testing.T) {
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
	var tenant, team, project, up, vs, epoch string
	if err := pool.QueryRow(ctx, `SELECT vmcp_tenant_id()`).Scan(&tenant); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO teams(slug,name,issuer,group_value) VALUES('owner','Owner','https://idp','owner') RETURNING id::text`).Scan(&team); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO projects(slug,name,owner_team_id) VALUES('owned','Owned',$1) RETURNING id::text`, team).Scan(&project); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO upstreams(tenant_id,slug,name,url,transport,owner_team_id) SELECT id,'provider','Provider','https://example.invalid','streamable_http',$1 FROM tenants WHERE slug='default' RETURNING id::text`, team).Scan(&up); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO virtual_servers(tenant_id,slug,name,project_id) SELECT id,'consumer','Consumer',$1 FROM tenants WHERE slug='default' RETURNING id::text`, project).Scan(&vs); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT epoch_id::text FROM audit_resource_epochs WHERE resource_kind='virtual_server' AND resource_id=$1 AND end_revision IS NULL`, vs).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	who := authz.Principal{Kind: "control_token", ID: "token"}
	for _, scope := range []authz.Scope{{Kind: "tenant"}, {Kind: "team", ID: team}, {Kind: "team", ID: "another-team"}, {Kind: "project", ID: project}, {Kind: "virtual_server", ID: vs}, {Kind: "virtual_server", ID: vs, Epoch: epoch}, {Kind: "virtual_server", ID: vs, Epoch: "old-epoch"}, {Kind: "upstream", ID: up}, {Kind: "unmanaged"}} {
		p := authz.Policy{Tenant: tenant, Revision: 1, Roles: []authz.Role{{ID: "role", Permissions: []string{"virtual_servers.read", "upstreams.read"}}}, Assignments: []authz.Assignment{{ID: "grant", RoleID: "role", Principal: who, Scope: scope, Enabled: true}}}
		for _, kind := range []string{"upstream", "virtual_server"} {
			r := authz.Resource{Tenant: tenant, Kind: kind, OwnerTeam: team}
			table, alias, permission := "upstreams", "u", "upstreams.read"
			r.ID = up
			if kind == "virtual_server" {
				table, alias, permission = "virtual_servers", "v", "virtual_servers.read"
				r.ID = vs
				r.Project = project
				r.Epoch = epoch
			} else {
				if err := pool.QueryRow(ctx, `SELECT epoch_id::text FROM audit_resource_epochs WHERE resource_kind='upstream' AND resource_id=$1 AND end_revision IS NULL`, up).Scan(&r.Epoch); err != nil {
					t.Fatal(err)
				}
			}
			expected := p.Decide(authz.Request{Principal: who, Permission: permission, Resource: r}).Allowed
			var actual bool
			sql := configurationPermissionSQL(p, who, permission, kind, alias)
			if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM `+table+` `+alias+` WHERE `+sql+`)`).Scan(&actual); err != nil {
				t.Fatal(scope, sql, err)
			}
			if actual != expected {
				t.Fatalf("%s %+v: SQL=%t core=%t", kind, scope, actual, expected)
			}
			p.Tenant = "other-deployment"
			if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM `+table+` `+alias+` WHERE `+configurationPermissionSQL(p, who, permission, kind, alias)+`)`).Scan(&actual); err != nil || actual {
				t.Fatal("cross-tenant predicate", actual, err)
			}
		}
	}
}
