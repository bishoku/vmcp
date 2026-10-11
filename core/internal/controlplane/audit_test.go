package controlplane_test

import (
	"context"
	"strings"
	"testing"

	"github.com/bishoku/vmcp/core/internal/audit"
)

func TestAdministrativeAuditAtomicityAndSecrets(t *testing.T) {
	a := setup(t)
	ctx := context.Background()
	headers, _ := a.expect(201, "POST", "/api/v1/upstreams", a.upstream())
	operation := headers.Get("X-Request-Id")
	if !audit.ValidID(operation) {
		t.Fatal("missing request identity")
	}
	var admission, mutation, outcome int
	var revision int64
	if err := a.pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE stage='admission'),count(*) FILTER(WHERE stage='mutation'),count(*) FILTER(WHERE stage='outcome'),max(config_revision) FROM audit_events WHERE operation_id=$1::uuid`, operation).Scan(&admission, &mutation, &outcome, &revision); err != nil || admission != 1 || mutation != 1 || outcome != 1 || revision < 2 {
		t.Fatalf("administrative phases: %d %d %d %d %v", admission, mutation, outcome, revision, err)
	}
	var target string
	if err := a.pool.QueryRow(ctx, `SELECT target FROM audit_events WHERE operation_id=$1::uuid AND stage='mutation'`, operation).Scan(&target); err != nil || target != "upstream:fx" {
		t.Fatalf("missing concrete mutation target: %s %v", target, err)
	}
	// Reject only the audit row inside the write transaction, after the data write ran.
	if _, err := a.pool.Exec(ctx, `CREATE FUNCTION fail_mutation_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.stage='mutation' THEN RAISE EXCEPTION 'audit disk full' USING ERRCODE='53100'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_mutation_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_mutation_audit()`); err != nil {
		t.Fatal(err)
	}
	a.expect(503, "PATCH", "/api/v1/upstreams/fx", map[string]any{"name": "MUST ROLLBACK"})
	var name string
	if err := a.pool.QueryRow(ctx, `SELECT name FROM upstreams WHERE slug='fx'`).Scan(&name); err != nil || name != "Fixture" {
		t.Fatalf("mutation escaped without transactional audit: %s %v", name, err)
	}
	if _, err := a.pool.Exec(ctx, `DROP TRIGGER fail_mutation_audit ON audit_events`); err != nil {
		t.Fatal(err)
	}
	// A privileged configuration export is admitted and acknowledged before its response.
	h, _ := a.expect(200, "GET", "/api/v1/export", nil)
	var phases int
	if err := a.pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE operation_id=$1::uuid`, h.Get("X-Request-Id")).Scan(&phases); err != nil || phases != 2 {
		t.Fatalf("export missing metadata audit: %d %v", phases, err)
	}
	_, tokenResponse := a.expect(201, "POST", "/api/v1/admin-tokens", map[string]any{"slug": "operator", "scope": "admin"})
	secret, _ := tokenResponse["token"].(string)
	if secret == "" {
		t.Fatal("missing issued token")
	}
	status, _, _ := a.do("PATCH", "/api/v1/upstreams/fx", map[string]any{"name": "Updated"}, map[string]string{"Authorization": "Bearer " + secret})
	if status != 200 {
		t.Fatalf("named admin request: %d", status)
	}
	var actor string
	if err := a.pool.QueryRow(ctx, `SELECT actor_id FROM audit_events WHERE actor_type='admin-token' AND stage='mutation' LIMIT 1`).Scan(&actor); err != nil || !audit.ValidID(actor) {
		t.Fatalf("named token not attributed: %s %v", actor, err)
	}
	rows, err := a.pool.Query(ctx, `SELECT row_to_json(e)::text FROM audit_events e`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(raw, "up-secret") || strings.Contains(raw, secret) || strings.Contains(raw, "MUST ROLLBACK") {
			t.Fatal("secret or payload entered audit")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	a.expect(200, "GET", "/api/v1/audit-status", nil)
	a.expect(200, "GET", "/api/v1/audit-events?stage=mutation&actorId="+actor, nil)
	a.expect(422, "GET", "/api/v1/audit-events?operationId=not-a-uuid", nil)
	a.expect(422, "GET", "/api/v1/audit-events?incomplete=not-bool", nil)
}

func TestFailedAdminAdmissionPreventsMutation(t *testing.T) {
	a := setup(t)
	ctx := context.Background()
	if _, err := a.pool.Exec(ctx, `CREATE FUNCTION fail_admission() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'full disk' USING ERRCODE='53100'; END $$; CREATE TRIGGER fail_admission BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_admission()`); err != nil {
		t.Fatal(err)
	}
	a.expect(503, "POST", "/api/v1/upstreams", a.upstream())
	var count int
	if err := a.pool.QueryRow(ctx, `SELECT count(*) FROM upstreams WHERE slug='fx'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("admin admission failure permitted mutation: %d %v", count, err)
	}
}
