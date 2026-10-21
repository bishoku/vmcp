package audit

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/secrets"
)

func payloadRuntime(t *testing.T) (*Runtime, context.CancelFunc) {
	t.Helper()
	pool := batchDatabase(t)
	box, err := secrets.NewBox(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := NewRuntime(ctx, pool, box, slog.New(slog.NewTextHandler(io.Discard, nil)), "strict")
	t.Cleanup(func() { cancel(); <-r.Done() })
	s := DefaultSettings()
	s.Mode = "strict"
	s.Revision = 9
	s.Scopes["virtual"] = Capture{true, true}
	s.Rules = []Rule{{"email", `[A-Za-z]+@example\.com`}}
	r.SetSettings(s)
	return r, cancel
}
func TestPayloadAtomicPersistenceAndPinnedMode(t *testing.T) {
	r, _ := payloadRuntime(t)
	ctx := context.Background()
	event, err := r.Admit(ctx, Event{Scope: "virtual", Boundary: "virtual", RawPayload: []byte(`{"email":"a@example.com","token":"never-store"}`)})
	if err != nil {
		t.Fatal(err)
	}
	var cipher []byte
	if err = r.Strict.Pool.QueryRow(ctx, `SELECT p.ciphertext FROM audit_payloads p JOIN audit_events a USING(event_id) WHERE a.event_id=$1`, event.EventID).Scan(&cipher); err != nil {
		t.Fatal(err)
	}
	raw, err := r.Box.OpenBound(cipher, event.EventID, "audit.payload")
	if err != nil || string(raw) != `{"email":"[REDACTED]","token":"[REDACTED]"}` {
		t.Fatal(string(raw), err)
	}
	// Changing settings cannot change the outcome guarantee or masking policy.
	s := DefaultSettings()
	s.Mode = "relax"
	s.Revision = 10
	r.SetSettings(s)
	if current := r.Settings(); current.Mode != "relax" || current.Revision != 10 {
		t.Fatal("new policy not applied", current)
	}
	event.Status = "ok"
	event.RawPayload = []byte(`{"email":"b@example.com"}`)
	if err = r.Complete(ctx, event); err != nil {
		t.Fatal(err)
	}
	var mode, state string
	var rev int
	if err = r.Strict.Pool.QueryRow(ctx, `SELECT audit_mode,payload_state,policy_revision FROM audit_events WHERE operation_id=$1 AND stage='outcome'`, event.OperationID).Scan(&mode, &state, &rev); err != nil || mode != "strict" || state != "redacted" || rev != 9 {
		t.Fatal(mode, state, rev, err)
	}
	// Encryption is bound to the event: copied ciphertext must not decrypt elsewhere.
	if _, err = r.Box.OpenBound(cipher, OutcomeID(event.OperationID), "audit.payload"); err == nil {
		t.Fatal("ciphertext context not enforced")
	}
	next, err := r.Admit(ctx, Event{Scope: "virtual", RawPayload: []byte(`{"email":"c@example.com"}`)})
	if err != nil || next.Mode != "relax" || next.PolicyRevision != 10 || next.PayloadState != "disabled" {
		t.Fatal("new operation did not use updated policy", next, err)
	}
}
func TestPayloadFailureRollsBackMetadataAndRelaxDoesNotBlock(t *testing.T) {
	r, cancel := payloadRuntime(t)
	ctx := context.Background()
	_, err := r.Strict.Pool.Exec(ctx, `CREATE FUNCTION reject_payload() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected payload write failure' USING ERRCODE='53100'; END $$; CREATE TRIGGER reject_payload BEFORE INSERT ON audit_payloads FOR EACH ROW EXECUTE FUNCTION reject_payload()`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Admit(ctx, Event{Scope: "virtual", RawPayload: []byte(`{"x":1}`)}); !errors.Is(err, ErrUnavailable) {
		t.Fatal("strict accepted failure", err)
	}
	var n int
	_ = r.Strict.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events`).Scan(&n)
	if n != 0 {
		t.Fatal("orphan metadata committed")
	}
	s := r.Settings()
	s.Mode = "relax"
	r.SetSettings(s)
	event, err := r.Admit(ctx, Event{Scope: "virtual", RawPayload: []byte(`{"x":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	event.Status = "ok"
	event.RawPayload = json.RawMessage(`{"ok":true}`)
	if err = r.Complete(ctx, event); err != nil {
		t.Fatal(err)
	}
	// Let the existing writer retry path stop promptly at shutdown in this test.
	r.Relax.shutdownTimeout = 50 * time.Millisecond
	cancel()
	<-r.Done()
	if r.Stats().Dropped == 0 {
		t.Fatal("lost relaxed events not counted")
	}
	_ = r.Strict.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events`).Scan(&n)
	if n != 0 {
		t.Fatal("relax committed partial pair")
	}
}
func TestPayloadIndependentExpiry(t *testing.T) {
	r, _ := payloadRuntime(t)
	ctx := context.Background()
	e, err := r.Admit(ctx, Event{Scope: "virtual", RawPayload: []byte(`{"x":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Strict.Pool.Exec(ctx, `UPDATE audit_payloads SET expires_at=now()-interval '1 second'`)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := PurgePayloads(ctx, r.Strict.Pool); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	var n int
	_ = r.Strict.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE event_id=$1`, e.EventID).Scan(&n)
	if n != 1 {
		t.Fatal("metadata expired with payload")
	}
}

func TestMetadataPurgeWaitsForPayloadCleanup(t *testing.T) {
	r, _ := payloadRuntime(t)
	ctx := context.Background()
	// A standalone event avoids altering immutable metadata to simulate age.
	if err := r.Write(ctx, Event{Time: time.Now().Add(-48 * time.Hour), Status: "ok", Scope: "virtual", Direction: "response", RawPayload: []byte(`{"x":1}`)}); err != nil {
		t.Fatal(err)
	}
	if n, err := Purge(ctx, r.Strict.Pool, 24*time.Hour); err != nil || n != 0 {
		t.Fatal("metadata purge bypassed payload retention", n, err)
	}
	var count int
	if err := r.Strict.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_payloads WHERE expires_at>now()`).Scan(&count); err != nil || count != 1 {
		t.Fatal("unexpired payload lost", count, err)
	}
	if _, err := r.Strict.Pool.Exec(ctx, `UPDATE audit_payloads SET expires_at=now()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	// Metadata deletion must defer to the payload purger even after expiry.
	if n, err := Purge(ctx, r.Strict.Pool, 24*time.Hour); err != nil || n != 0 {
		t.Fatal("metadata bypassed payload cleanup", n, err)
	}
	if n, err := PurgePayloads(ctx, r.Strict.Pool); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if n, err := Purge(ctx, r.Strict.Pool, 24*time.Hour); err != nil || n != 1 {
		t.Fatal("metadata not released after payload cleanup", n, err)
	}
}

type payloadPublisher struct {
	bodies [][]byte
	fail   bool
}

func (p *payloadPublisher) Publish(_ context.Context, _ string, body []byte) error {
	p.bodies = append(p.bodies, append([]byte(nil), body...))
	if p.fail {
		return errors.New("temporary failure")
	}
	return nil
}
func (*payloadPublisher) Close() {}
func TestPayloadExportStableAcrossRetryAndExpiry(t *testing.T) {
	r, _ := payloadRuntime(t)
	ctx := context.Background()
	e, err := r.Admit(ctx, Event{Scope: "virtual", RawPayload: []byte(`{"email":"a@example.com","token":"never-export","content":[{"type":"text","text":"{\"token\":\"never-export\"}"}]}`)})
	if err != nil {
		t.Fatal(err)
	}
	publisher := &payloadPublisher{fail: true}
	exporter, err := NewPublishingExporter(r.Strict.Pool, "payload-test", publisher)
	if err != nil {
		t.Fatal(err)
	}
	exporter.Box = r.Box
	if err = exporter.Register(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = r.Strict.Pool.Exec(ctx, `UPDATE audit_export_state SET include_payload=true WHERE consumer='payload-test'`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = exporter.Step(ctx); err != nil {
		t.Fatal(err)
	}
	if len(publisher.bodies) != 1 {
		t.Fatal("no export")
	}
	if !json.Valid(publisher.bodies[0]) {
		t.Fatal("invalid export")
	}
	var envelope map[string]any
	_ = json.Unmarshal(publisher.bodies[0], &envelope)
	if envelope["payload"] == nil {
		t.Fatal("payload missing")
	}
	if bytes.Contains(publisher.bodies[0], []byte("never-export")) || bytes.Contains(publisher.bodies[0], []byte("a@example.com")) {
		t.Fatal("export contains an unmasked secret")
	}
	_, err = r.Strict.Pool.Exec(ctx, `DELETE FROM audit_payloads WHERE event_id=$1`, e.EventID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Strict.Pool.Exec(ctx, `UPDATE audit_deliveries SET next_attempt=now() WHERE consumer='payload-test'`)
	if err != nil {
		t.Fatal(err)
	}
	publisher.fail = false
	if _, err = exporter.Step(ctx); err != nil {
		t.Fatal(err)
	}
	if len(publisher.bodies) != 2 || string(publisher.bodies[0]) != string(publisher.bodies[1]) {
		t.Fatal("retry envelope changed")
	}
	var saved []byte
	if err = r.Strict.Pool.QueryRow(ctx, `SELECT envelope FROM audit_deliveries WHERE consumer='payload-test' AND event_id=$1`, e.EventID).Scan(&saved); err != nil || len(saved) != 0 {
		t.Fatal("delivered payload snapshot retained", err)
	}
}
