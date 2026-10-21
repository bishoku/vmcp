package virtual

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"strings"
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

func TestRuntimePayloadsOnVirtualAndUpstreamBoundaries(t *testing.T) {
	for _, mode := range []string{"strict", "relax"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			url := testdb.New(t)
			if err := store.Migrate(ctx, url); err != nil {
				t.Fatal(err)
			}
			pool, err := store.Connect(ctx, url)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(pool.Close)
			box, err := secrets.NewBox(base64.StdEncoding.EncodeToString(make([]byte, 32)))
			if err != nil {
				t.Fatal(err)
			}
			writerCtx, cancel := context.WithCancel(ctx)
			r := newRig(t)
			runtime := audit.NewRuntime(writerCtx, pool, box, r.handler.Log, "strict")
			t.Cleanup(func() { cancel(); <-runtime.Done() })
			r.handler.Audit = runtime
			policy := audit.DefaultSettings()
			policy.Mode = mode
			policy.Scopes["virtual"] = audit.Capture{Request: true, Response: true}
			policy.Scopes["upstream"] = audit.Capture{Request: true, Response: true}
			policy.Rules = []audit.Rule{{Name: "email", Pattern: `[A-Za-z]+@example\.com`}}
			policy.VirtualServers["support"] = []audit.Rule{{Name: "customer", Pattern: `CUST-[0-9]+`}}
			policy.Upstreams["github"] = []audit.Rule{{Name: "employee", Pattern: `EMP-[0-9]+`}}
			r.handler.Snap.(staticSnap).s.Audit = policy
			runtime.SetSettings(policy)
			sid := r.initialize(t)
			response, msg := r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: "github__create_issue", Arguments: json.RawMessage(`{"email":"a@example.com","customer":"CUST-123","employee":"EMP-456","token":"never-store"}`)})
			if response.StatusCode != 200 || msg == nil || msg.Error != nil {
				t.Fatalf("call failed: %d %+v", response.StatusCode, msg)
			}
			cancel()
			<-runtime.Done()
			rows, err := pool.Query(ctx, `SELECT a.event_id::text,a.boundary,a.direction,p.ciphertext FROM audit_events a JOIN audit_payloads p USING(event_id) WHERE a.method='tools/call' ORDER BY a.id`)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			seen := map[string]bool{}
			for rows.Next() {
				var id, boundary, direction string
				var cipher []byte
				if err := rows.Scan(&id, &boundary, &direction, &cipher); err != nil {
					t.Fatal(err)
				}
				raw, err := box.OpenBound(cipher, id, "audit.payload")
				if err != nil {
					t.Fatal(err)
				}
				if direction == "request" {
					for _, secret := range []string{"a@example.com", "CUST-123", "EMP-456", "never-store"} {
						if strings.Contains(string(raw), secret) {
							t.Fatal("unmasked request", string(raw))
						}
					}
				}
				seen[boundary+":"+direction] = true
			}
			for _, key := range []string{"virtual:request", "virtual:response", "upstream:request", "upstream:response"} {
				if !seen[key] {
					t.Fatal("missing capture", key, seen)
				}
			}
		})
	}
}
