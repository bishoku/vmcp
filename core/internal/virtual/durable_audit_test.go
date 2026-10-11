package virtual

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
)

func TestStrictAuditAdmissionBeforeSideEffects(t *testing.T) {
	for _, mode := range []string{"success", "disk_full", "outcome_failure", "database_outage"} {
		t.Run(mode, func(t *testing.T) {
			url := testdb.New(t)
			ctx := context.Background()
			if err := store.Migrate(ctx, url); err != nil {
				t.Fatal(err)
			}
			pool, err := store.Connect(ctx, url)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(pool.Close)
			r := newRig(t)
			d := audit.NewDurable(pool, r.handler.Log)
			r.handler.Audit = d
			sid := r.initialize(t)
			switch mode {
			case "disk_full", "outcome_failure":
				stage := "admission"
				if mode == "outcome_failure" {
					stage = "outcome"
				}
				_, err := pool.Exec(ctx, `CREATE FUNCTION reject_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.stage='`+stage+`' THEN RAISE EXCEPTION 'injected disk failure' USING ERRCODE='53100'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_audit()`)
				if err != nil {
					t.Fatal(err)
				}
			case "database_outage":
				pool.Close()
			}
			response, msg := r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: "github__create_issue", Arguments: json.RawMessage(`{"secret":"never-audit-this-payload"}`)})
			r.github.mu.Lock()
			inits := r.github.inits
			r.github.mu.Unlock()
			if mode == "disk_full" || mode == "database_outage" {
				if response.StatusCode != 503 || inits != 0 || d.Stats().BlockedAdmissions != 1 {
					t.Fatalf("unacknowledged side effect: status=%d inits=%d stats=%+v", response.StatusCode, inits, d.Stats())
				}
				return
			}
			pending, err := audit.Pending(ctx, pool)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "outcome_failure" {
				if response.StatusCode != 503 || inits != 1 || pending.Incomplete != 1 {
					t.Fatalf("invented completion: status=%d inits=%d pending=%+v", response.StatusCode, inits, pending)
				}
				return
			}
			if response.StatusCode != 200 || msg == nil || msg.Error != nil || pending.Incomplete != 0 {
				t.Fatalf("successful call: %d %+v %+v", response.StatusCode, msg, pending)
			}
			var admissions, outcomes int
			var correlation bool
			if err := pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE stage='admission'),count(*) FILTER(WHERE stage='outcome'),count(DISTINCT operation_id)=1 AND count(DISTINCT request_id)=1 FROM audit_events WHERE method='tools/call'`).Scan(&admissions, &outcomes, &correlation); err != nil || admissions != 1 || outcomes != 1 || !correlation {
				t.Fatalf("missing phases: %d %d %v %v", admissions, outcomes, correlation, err)
			}
			var leaked int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events e WHERE row_to_json(e)::text LIKE '%never-audit-this-payload%'`).Scan(&leaked); err != nil || leaked != 0 {
				t.Fatalf("payload leaked: %d %v", leaked, err)
			}
			if !audit.ValidID(response.Header.Get("X-Request-Id")) {
				t.Fatal("missing safe correlation ID")
			}
		})
	}
}
