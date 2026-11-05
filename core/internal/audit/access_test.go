package audit

import (
	"context"
	"testing"
)

func tenantAccess() *AccessContext {
	return &AccessContext{Tenant: "default", Classification: "tenant", Origin: "test", Revision: 1}
}
func TestAuditAccessIsOperationOwnedAndRevisionPinned(t *testing.T) {
	source := tenantAccess()
	e, err := Prepare(Event{Status: "ok", Access: source})
	if err != nil {
		t.Fatal(err)
	}
	source.Tenant = "changed"
	if e.Access.Tenant != "default" || e.Revision != 1 {
		t.Fatal("context alias or revision drift")
	}
	e.Revision = 2
	if _, err = Prepare(e); err == nil {
		t.Fatal("mismatched snapshot revision accepted")
	}
	e = Event{Status: "ok", Access: &AccessContext{Tenant: "default", Classification: "resource", Origin: "virtual", Revision: 1}}
	if _, err = Prepare(e); err == nil {
		t.Fatal("resource provenance without identity accepted")
	}
}
func TestProvenanceBatchAtomicityAndLegacyReplay(t *testing.T) {
	pool := batchDatabase(t)
	ctx := context.Background()
	batch := batchEvents(t, 2)
	batch[0].event.Access = tenantAccess()
	batch[1].event.Access = tenantAccess()
	batch[1].event.Access.Tenant = "wrong-tenant"
	if err := persistBatch(ctx, pool, batch); err == nil {
		t.Fatal("invalid provenance accepted")
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events`).Scan(&n); err != nil || n != 0 {
		t.Fatal("partial event/context committed", n, err)
	}
	batch[1].event.Access.Tenant = "default"
	if err := persistBatch(ctx, pool, batch); err != nil {
		t.Fatal(err)
	}
	if err := persistBatch(ctx, pool, batch); err != nil {
		t.Fatal("replay", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_access_contexts`).Scan(&n); err != nil || n != 2 {
		t.Fatal("duplicate context", n, err)
	}
	legacy := batchEvents(t, 1)
	if err := persistBatch(ctx, pool, legacy); err != nil {
		t.Fatal(err)
	}
	legacy[0].event.Access = tenantAccess()
	if err := persistBatch(ctx, pool, legacy); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_access_contexts WHERE event_id=$1`, legacy[0].event.EventID).Scan(&n); err != nil || n != 0 {
		t.Fatal("replay backfilled legacy evidence", n, err)
	}
}

func TestBulkProvenanceDuplicateWithinBatchKeepsFirstContext(t *testing.T) {
	pool := batchDatabase(t)
	ctx := context.Background()
	batch := batchEvents(t, 2)
	batch[0].event.Access = tenantAccess()
	duplicate := *batch[0]
	duplicate.event.Access = tenantAccess()
	duplicate.event.Access.Origin = "must-not-replace-first"
	batch = append(batch, &duplicate)
	if err := persistBatch(ctx, pool, batch); err != nil {
		t.Fatal(err)
	}
	var count int
	var origin string
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events`).Scan(&count); err != nil || count != 2 {
		t.Fatal(count, err)
	}
	if err := pool.QueryRow(ctx, `SELECT origin FROM audit_access_contexts WHERE event_id=$1`, batch[0].event.EventID).Scan(&origin); err != nil || origin != "test" {
		t.Fatal(origin, err)
	}
}

func TestBulkProvenanceDomainFailureRollsBackEventsAndContexts(t *testing.T) {
	pool := batchDatabase(t)
	ctx := context.Background()
	batch := batchEvents(t, 2)
	for _, r := range batch {
		r.event.Access = tenantAccess()
	}
	resource, _ := NewID()
	epoch, _ := NewID()
	batch[1].event.Access = &AccessContext{Tenant: "default", Classification: "resource", Origin: "virtual", Revision: 1, Domains: []AccessDomain{{Kind: "consumer", ResourceKind: "virtual_server", ResourceID: resource, EpochID: epoch}}}
	if err := persistBatch(ctx, pool, batch); err == nil {
		t.Fatal("missing epoch accepted")
	}
	for _, table := range []string{"audit_events", "audit_access_contexts", "audit_access_domains"} {
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil || count != 0 {
			t.Fatal(table, count, err)
		}
	}
}

func TestBulkPayloadFailureRollsBackWholeBatch(t *testing.T) {
	pool := batchDatabase(t)
	ctx := context.Background()
	batch := batchEvents(t, 2)
	for _, r := range batch {
		r.event.Access = tenantAccess()
		r.event.Ciphertext = []byte("opaque-encrypted-payload")
	}
	if _, err := pool.Exec(ctx, `CREATE FUNCTION reject_second_payload() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF EXISTS(SELECT 1 FROM audit_payloads) THEN RAISE EXCEPTION 'injected second payload failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_second_payload BEFORE INSERT ON audit_payloads FOR EACH ROW EXECUTE FUNCTION reject_second_payload()`); err != nil {
		t.Fatal(err)
	}
	if err := persistBatch(ctx, pool, batch); err == nil {
		t.Fatal("payload failure accepted")
	}
	for _, table := range []string{"audit_events", "audit_access_contexts", "audit_payloads"} {
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil || n != 0 {
			t.Fatal("partial batch persisted", table, n, err)
		}
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER reject_second_payload ON audit_payloads`); err != nil {
		t.Fatal(err)
	}
	if err := persistBatch(ctx, pool, batch); err != nil {
		t.Fatal(err)
	}
	if err := persistBatch(ctx, pool, batch); err != nil {
		t.Fatal("replay", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events JOIN audit_access_contexts USING(event_id) JOIN audit_payloads USING(event_id)`).Scan(&n); err != nil || n != 2 {
		t.Fatal("payload/context association or replay failed", n, err)
	}
}
