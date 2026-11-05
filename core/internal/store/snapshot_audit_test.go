package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"github.com/pressly/goose/v3"
	"io"
	"log/slog"
	"testing"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/authz"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/testdb"
)

func TestSnapshotAuditOwnershipTransferAndHistoricalAccess(t *testing.T) {
	for _, mode := range []string{"strict", "relax"} {
		t.Run(mode, func(t *testing.T) {
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
			exec := func(sql string, args ...any) {
				t.Helper()
				if _, err := pool.Exec(ctx, sql, args...); err != nil {
					t.Fatal(err)
				}
			}
			var teamA, teamB, project, upID, vsID string
			for _, pair := range []struct {
				name string
				dst  *string
			}{{"a", &teamA}, {"b", &teamB}} {
				if err = pool.QueryRow(ctx, `INSERT INTO teams(slug,name,issuer,group_value) VALUES($1,$1,'https://idp',$1) RETURNING id::text`, pair.name).Scan(pair.dst); err != nil {
					t.Fatal(err)
				}
			}
			if err = pool.QueryRow(ctx, `INSERT INTO projects(slug,name,owner_team_id) VALUES('project','Project',$1) RETURNING id::text`, teamA).Scan(&project); err != nil {
				t.Fatal(err)
			}
			if err = pool.QueryRow(ctx, `INSERT INTO upstreams(tenant_id,slug,name,url,transport,owner_team_id) SELECT id,'provider','Provider','https://example.invalid','streamable_http',$1 FROM tenants WHERE slug='default' RETURNING id::text`, teamB).Scan(&upID); err != nil {
				t.Fatal(err)
			}
			if err = pool.QueryRow(ctx, `INSERT INTO virtual_servers(tenant_id,slug,name,project_id) SELECT id,'chat','Chat',$1 FROM tenants WHERE slug='default' RETURNING id::text`, project).Scan(&vsID); err != nil {
				t.Fatal(err)
			}
			box, _ := secrets.NewBox(base64.StdEncoding.EncodeToString(make([]byte, 32)))
			snap, err := LoadSnapshot(ctx, pool, box)
			if err != nil {
				t.Fatal(err)
			}
			old := snap.VirtualServers["chat"].AuditDomain
			if old.OwnerTeamID != teamA || old.ProjectID != project || old.ResourceID != vsID || old.EpochID == "" {
				t.Fatalf("snapshot domain %+v", old)
			}
			var roleID string
			if err = pool.QueryRow(ctx, `SELECT id::text FROM authorization_roles WHERE slug='observer'`).Scan(&roleID); err != nil {
				t.Fatal(err)
			}
			exec(`INSERT INTO authorization_assignments(role_id,principal_kind,principal_id,scope_kind,scope_id) VALUES($1,'control_token','live-test','virtual_server',$2)`, roleID, vsID)
			exec(`INSERT INTO authorization_assignments(role_id,principal_kind,principal_id,scope_kind,scope_id,epoch_id) VALUES($1,'control_token','history-test','virtual_server',$2,$3)`, roleID, vsID, old.EpochID)
			writerCtx, cancel := context.WithCancel(ctx)
			r := audit.NewRuntime(writerCtx, pool, box, slog.New(slog.NewTextHandler(io.Discard, nil)), "strict")
			defer func() { cancel(); <-r.Done() }()
			settings := audit.DefaultSettings()
			settings.Mode = mode
			settings.Scopes["virtual"] = audit.Capture{Request: true, Response: true}
			settings.Scopes["upstream"] = audit.Capture{Request: true, Response: true}
			r.SetSettings(settings)
			ev, err := r.Admit(ctx, audit.Event{Access: snap.ConsumerAudit(snap.VirtualServers["chat"]), Revision: snap.Revision, Scope: "virtual", Method: "tools/call", RawPayload: []byte(`{"message":"request"}`)})
			if err != nil {
				t.Fatal(err)
			}
			op := audit.WithProvider(audit.WithOperation(ctx, r, ev), "provider", snap.Upstreams[upID].AuditDomain)
			finish, err := audit.Observe(op, "tools/call", map[string]string{"message": "upstream"})
			if err != nil {
				t.Fatal(err)
			}
			// Change ownership while both the virtual and upstream operations are in flight.
			exec(`UPDATE projects SET owner_team_id=$1,version=version+1 WHERE id=$2`, teamB, project)
			newer, err := LoadSnapshotWithPrevious(ctx, pool, box, snap)
			if err != nil {
				t.Fatal(err)
			}
			var liveEnabled, historyEnabled bool
			if err = pool.QueryRow(ctx, `SELECT enabled FROM authorization_assignments WHERE principal_id='live-test'`).Scan(&liveEnabled); err != nil {
				t.Fatal(err)
			}
			if err = pool.QueryRow(ctx, `SELECT enabled FROM authorization_assignments WHERE principal_id='history-test'`).Scan(&historyEnabled); err != nil {
				t.Fatal(err)
			}
			if liveEnabled || !historyEnabled {
				t.Fatal("transfer confused live and historical assignments")
			}
			fresh := newer.VirtualServers["chat"].AuditDomain
			if fresh.EpochID == old.EpochID || fresh.OwnerTeamID != teamB || snap.VirtualServers["chat"].AuditDomain != old {
				t.Fatal("transfer did not isolate immutable snapshots")
			}
			if err = finish([]byte(`{"message":"upstream response"}`), nil); err != nil {
				t.Fatal(err)
			}
			ev.Status = "ok"
			ev.RawPayload = []byte(`{"message":"response"}`)
			if err = r.Complete(ctx, ev); err != nil {
				t.Fatal(err)
			}
			latest, err := r.Admit(ctx, audit.Event{Access: newer.ConsumerAudit(newer.VirtualServers["chat"]), Revision: newer.Revision, Scope: "virtual", Method: "new-call", RawPayload: []byte(`{"new":true}`)})
			if err != nil {
				t.Fatal(err)
			}
			latest.Status = "ok"
			latest.RawPayload = []byte(`{"ok":true}`)
			if err = r.Complete(ctx, latest); err != nil {
				t.Fatal(err)
			}
			cancel()
			<-r.Done()
			st := Store{Pool: pool, Box: box}
			who := authz.Principal{Kind: "oidc", Issuer: "https://idp", ID: "investigator"}
			policy := authz.Policy{Tenant: snap.AuditTenant, Revision: 1, Roles: []authz.Role{{ID: "r", Permissions: []string{"audit.events.read", "audit.payloads.request.read", "audit.payloads.response.read"}}}, Assignments: []authz.Assignment{{ID: "a", RoleID: "r", Principal: who, Scope: authz.Scope{Kind: "team", ID: teamA}, CaptureScopes: []string{"virtual", "upstream"}, Enabled: true}}}
			for _, eid := range []string{ev.EventID, audit.OutcomeID(ev.OperationID)} {
				access, err := st.AuditPayloadAccess(ctx, eid)
				if err != nil {
					t.Fatal(err)
				}
				if !policy.CanReadPayload(who, access) {
					t.Fatal("old team lost historical operation")
				}
				policy.Assignments[0].Scope.ID = teamB
				if policy.CanReadPayload(who, access) {
					t.Fatal("new owner inherited old payload")
				}
				policy.Assignments[0].Scope.ID = teamA
			}
			access, err := st.AuditPayloadAccess(ctx, latest.EventID)
			if err != nil {
				t.Fatal(err)
			}
			if policy.CanReadPayload(who, access) {
				t.Fatal("old team read new call")
			}
			policy.Assignments[0].Scope.ID = teamB
			if !policy.CanReadPayload(who, access) {
				t.Fatal("new team cannot read new call")
			}
			var count int
			if err = pool.QueryRow(ctx, `SELECT count(*) FROM audit_events e JOIN audit_access_contexts c USING(event_id) JOIN audit_access_domains d USING(event_id) WHERE (e.operation_id=$1 OR e.parent_operation_id=$1) AND d.domain_kind='consumer' AND d.epoch_id=$2 AND c.operation_config_revision=$3`, ev.OperationID, old.EpochID, snap.Revision).Scan(&count); err != nil || count != 4 {
				t.Fatal("request/response or child context changed", count, err)
			}
			if err = pool.QueryRow(ctx, `SELECT count(*) FROM audit_events e JOIN audit_access_domains d USING(event_id) WHERE e.parent_operation_id=$1 AND d.domain_kind='provider' AND d.resource_id=$2`, ev.OperationID, upID).Scan(&count); err != nil || count != 2 {
				t.Fatal("provider evidence missing", count, err)
			}
			previousProvider := snap.Upstreams[upID].AuditDomain
			exec(`UPDATE upstreams SET owner_team_id=$1 WHERE id=$2`, teamA, upID)
			movedProvider, err := LoadSnapshot(ctx, pool, box)
			if err != nil {
				t.Fatal(err)
			}
			if movedProvider.Upstreams[upID].AuditDomain.EpochID == previousProvider.EpochID || movedProvider.Upstreams[upID].AuditDomain.OwnerTeamID != teamA {
				t.Fatal("upstream transfer did not rotate epoch")
			}
			if err = pool.QueryRow(ctx, `SELECT count(*) FROM audit_events e JOIN audit_access_domains d USING(event_id) WHERE e.parent_operation_id=$1 AND d.domain_kind='provider' AND d.epoch_id=$2 AND d.owner_team_id=$3`, ev.OperationID, previousProvider.EpochID, teamB).Scan(&count); err != nil || count != 2 {
				t.Fatal("upstream transfer rewrote old provider evidence", count, err)
			}
			// Rollback never creates a new epoch; metadata edits do not rotate ownership.
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = tx.Exec(ctx, `UPDATE projects SET owner_team_id=$1 WHERE id=$2`, teamA, project); err != nil {
				t.Fatal(err)
			}
			_ = tx.Rollback(ctx)
			exec(`UPDATE virtual_servers SET name='Renamed' WHERE id=$1`, vsID)
			after, err := LoadSnapshot(ctx, pool, box)
			if err != nil || after.VirtualServers["chat"].AuditDomain.EpochID != fresh.EpochID {
				t.Fatal("metadata/rollback rotated epoch", err)
			}
			// Deleting/recreating a slug cannot inherit its old UUID/epoch.
			exec(`DELETE FROM virtual_servers WHERE id=$1`, vsID)
			exec(`INSERT INTO virtual_servers(tenant_id,slug,name,project_id) SELECT id,'chat','Recreated',$1 FROM tenants WHERE slug='default'`, project)
			recreated, err := LoadSnapshot(ctx, pool, box)
			if err != nil {
				t.Fatal(err)
			}
			if recreated.VirtualServers["chat"].AuditDomain.ResourceID == vsID || recreated.VirtualServers["chat"].AuditDomain.EpochID == fresh.EpochID {
				t.Fatal("slug reuse inherited historical identity")
			}
		})
	}
}

func TestAuditEpochMigrationDownUpPreservesEvidence(t *testing.T) {
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
	if _, err = pool.Exec(ctx, `INSERT INTO virtual_servers(tenant_id,slug,name) SELECT id,'retained','Retained' FROM tenants WHERE slug='default'`); err != nil {
		t.Fatal(err)
	}
	box, _ := secrets.NewBox(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	old, err := LoadSnapshot(ctx, pool, box)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := audit.NewID()
	if err = audit.Persist(ctx, pool, audit.Event{EventID: id, Status: "ok", Access: old.ConsumerAudit(old.VirtualServers["retained"])}); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	migrationMu.Lock()
	err = goose.DownToContext(ctx, db, ".", 37)
	migrationMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	fresh, err := LoadSnapshot(ctx, pool, box)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.VirtualServers["retained"].AuditDomain != old.VirtualServers["retained"].AuditDomain {
		t.Fatal("re-upgrade rewrote existing epoch")
	}
	access, err := (&Store{Pool: pool}).AuditPayloadAccess(ctx, id)
	if err != nil || access.Classification != "resource" || len(access.Domains) != 1 {
		t.Fatal("evidence lost on migration roundtrip", access, err)
	}
}
