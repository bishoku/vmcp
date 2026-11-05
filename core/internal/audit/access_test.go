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
