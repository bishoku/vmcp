package audit

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
)

// Previous per-event pipeline is a measurement reference, never a runtime path.
func provenancePipelineReference(ctx context.Context, pool *pgxpool.Pool, batch []*persistReq) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `SET LOCAL synchronous_commit=on`); err != nil {
		return err
	}
	var fallbackRev int64
	prepared := make([]Event, len(batch))
	statements := &pgx.Batch{}
	for i, r := range batch {
		e, err := Prepare(r.event)
		if err != nil {
			return err
		}
		if e.Revision == 0 {
			if fallbackRev == 0 {
				if err = tx.QueryRow(ctx, `SELECT revision FROM config_revision`).Scan(&fallbackRev); err != nil {
					return err
				}
			}
			e.Revision = fallbackRev
		}
		args, err := insertArgs(e)
		if err != nil {
			return err
		}
		prepared[i] = e
		statements.Queue(insertSQL, args...)
	}
	results := tx.SendBatch(ctx, statements)
	inserted := make([]bool, len(prepared))
	for i := range prepared {
		tag, err := results.Exec()
		if err != nil {
			_ = results.Close()
			return err
		}
		inserted[i] = tag.RowsAffected() > 0
	}
	if err = results.Close(); err != nil {
		return err
	}
	// Only rows inserted in this transaction may gain context or payload. This
	// keeps replay idempotent and cannot backfill ownership onto legacy evidence.
	dependents := &pgx.Batch{}
	for i, e := range prepared {
		if !inserted[i] {
			continue
		}
		queueAccess(dependents, e)
		if len(e.Ciphertext) > 0 {
			dependents.Queue(`INSERT INTO audit_payloads(event_id,ciphertext,expires_at) VALUES($1,$2,$3)`, e.EventID, e.Ciphertext, e.PayloadExpiresAt)
		}
	}
	if dependents.Len() > 0 {
		results = tx.SendBatch(ctx, dependents)
		for range dependents.Len() {
			if _, err = results.Exec(); err != nil {
				_ = results.Close()
				return err
			}
		}
		if err = results.Close(); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func BenchmarkProvenanceBatch(b *testing.B) {
	pool := batchDatabase(b)
	ctx := context.Background()
	resource, err := NewID()
	if err != nil {
		b.Fatal(err)
	}
	var epoch string
	if err := pool.QueryRow(ctx, `INSERT INTO audit_resource_epochs(resource_kind,resource_id,start_revision) VALUES('virtual_server',$1,1) RETURNING epoch_id::text`, resource).Scan(&epoch); err != nil {
		b.Fatal(err)
	}
	access := &AccessContext{Tenant: "default", Classification: "resource", Origin: "virtual", Revision: 1, Domains: []AccessDomain{{Kind: "consumer", ResourceKind: "virtual_server", ResourceID: resource, EpochID: epoch}}}
	for _, method := range []string{"pipeline", "bulk"} {
		b.Run(method, func(b *testing.B) {
			b.ReportAllocs()
			b.ReportMetric(64, "events/batch")
			for b.Loop() {
				b.StopTimer()
				batch := batchEvents(b, 64)
				for _, r := range batch {
					r.event.Access = access
				}
				b.StartTimer()
				var err error
				if method == "pipeline" {
					err = provenancePipelineReference(ctx, pool, batch)
				} else {
					err = persistBatch(ctx, pool, batch)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
