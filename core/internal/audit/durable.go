package audit

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const WriteTimeout = 5 * time.Second

// DurableRecorder acknowledges PostgreSQL admission before external side effects.
// Outcomes are separate facts; an absent outcome means unknown, never success.
type DurableRecorder interface {
	Recorder
	Admit(context.Context, Event) (Event, error)
	Complete(context.Context, Event) error
}

// NewID produces server-owned UUIDs, never secrets or caller-supplied correlation text.
func NewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var tracePattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// TraceID accepts only the trace ID from a valid version-00 W3C traceparent.
func TraceID(parent string) string {
	parts := strings.Split(parent, "-")
	if len(parts) != 4 || parts[0] != "00" || !tracePattern.MatchString(parts[1]) || parts[1] == strings.Repeat("0", 32) || len(parts[2]) != 16 || len(parts[3]) != 2 {
		return ""
	}
	for _, v := range []string{parts[2], parts[3]} {
		for _, c := range v {
			if !strings.ContainsRune("0123456789abcdef", c) {
				return ""
			}
		}
	}
	if parts[2] == strings.Repeat("0", 16) {
		return ""
	}
	return parts[1]
}

func Prepare(e Event) (Event, error) {
	var err error
	if e.EventID == "" {
		e.EventID, err = NewID()
		if err != nil {
			return e, err
		}
	}
	if !uuidPattern.MatchString(e.EventID) {
		return e, errors.New("invalid audit event ID")
	}
	for _, id := range []string{e.OperationID, e.RequestID} {
		if id != "" && !uuidPattern.MatchString(id) {
			return e, errors.New("invalid audit correlation ID")
		}
	}
	if e.TraceID != "" && !tracePattern.MatchString(e.TraceID) {
		return e, errors.New("invalid audit trace ID")
	}
	if e.RequestID == "" {
		e.RequestID, err = NewID()
		if err != nil {
			return e, err
		}
	}
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	if e.Mode == "" {
		e.Mode = "strict"
	}
	if e.PayloadState == "" {
		e.PayloadState = "disabled"
	}
	if e.SchemaVersion == 0 {
		e.SchemaVersion = 1
	}
	if e.Stage == "" {
		e.Stage = "event"
	}
	switch e.Stage {
	case "event", "admission", "outcome", "mutation":
	default:
		return e, errors.New("invalid audit stage")
	}
	switch e.Status {
	case "started", "ok", "tool_error", "error", "denied":
	default:
		return e, errors.New("invalid audit status")
	}
	if (e.Stage == "admission" || e.Stage == "outcome") && e.OperationID == "" {
		return e, errors.New("audit lifecycle requires operation ID")
	}
	if e.Stage == "admission" && e.Status != "started" || e.Stage == "outcome" && e.Status == "started" {
		return e, errors.New("invalid audit lifecycle status")
	}
	if e.Action == "" {
		e.Action = e.Method
	}
	for _, v := range []string{e.VirtualServer, e.AgentKey, e.ClientIP, e.Subject, e.Method, e.Tool, e.Upstream, e.Reason, e.ActorType, e.ActorID, e.Issuer, e.Action, e.Target} {
		if len(v) > 2048 {
			return e, errors.New("audit metadata too long")
		}
	}
	return e, nil
}

// Sink is shared by a pool and a transaction. Insert never stores request bodies or credentials.
type Sink interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

const insertSQL = `INSERT INTO audit_events
 (event_id,operation_id,request_id,trace_id,schema_version,actor_type,actor_id,issuer,action,target,config_revision,stage,
 ts,virtual_server,agent_key,client_ip,method,subject,tool,upstream,status,reason,duration_ms,redactions,audit_mode,policy_revision,scope,boundary,direction,parent_operation_id,payload_state,payload_original_bytes,payload_stored_bytes,payload_redactions)
 VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30::uuid,$31,$32,$33,$34)
 ON CONFLICT(event_id) DO NOTHING`

func insertArgs(e Event) ([]any, error) {
	e, err := Prepare(e)
	if err != nil {
		return nil, err
	}
	return []any{e.EventID, nullable(e.OperationID), nullable(e.RequestID), e.TraceID, e.SchemaVersion, e.ActorType, e.ActorID, e.Issuer, e.Action, e.Target, e.Revision, e.Stage, e.Time, e.VirtualServer, nullable(e.AgentKey), nullable(e.ClientIP), e.Method, nullable(e.Subject), nullable(e.Tool), nullable(e.Upstream), e.Status, nullable(e.Reason), e.DurationMS, e.Redactions, e.Mode, e.PolicyRevision, e.Scope, e.Boundary, e.Direction, nullable(e.ParentOperationID), e.PayloadState, e.PayloadOriginalBytes, e.PayloadStoredBytes, e.PayloadRedactions}, nil
}
func Insert(ctx context.Context, q Sink, e Event) error {
	e, err := Prepare(e)
	if err != nil {
		return err
	}
	args, err := insertArgs(e)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, insertSQL, args...)
	if err != nil {
		return err
	}
	if len(e.Ciphertext) > 0 {
		_, err = q.Exec(ctx, `INSERT INTO audit_payloads(event_id,ciphertext,expires_at) VALUES($1::uuid,$2,$3) ON CONFLICT(event_id) DO NOTHING`, e.EventID, e.Ciphertext, e.PayloadExpiresAt)
	}
	return err
}

// Persist acknowledges local PostgreSQL WAL durability, independent of session defaults.
func Persist(ctx context.Context, pool *pgxpool.Pool, e Event) error {
	if pool == nil {
		return errors.New("audit database pool is nil")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `SET LOCAL synchronous_commit=on`); err != nil {
		return err
	}
	if e.Revision == 0 {
		if err = tx.QueryRow(ctx, `SELECT revision FROM config_revision`).Scan(&e.Revision); err != nil {
			return err
		}
	}
	if err = Insert(ctx, tx, e); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func transient(err error) bool {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return strings.HasPrefix(pe.Code, "08") || pe.Code == "40001" || pe.Code == "40P01" || pe.Code == "57P01" || pe.Code == "57P02" || pe.Code == "57P03" || pe.Code == "53300"
	}
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

// Counters describe process-local failures. Durable incomplete operations are queried separately.
type Counters struct {
	PolicyRevision          int64   `json:"policyRevision"`
	PendingBytes            int64   `json:"pendingBytes"`
	Mode                    string  `json:"mode"`
	BlockedAdmissions       int64   `json:"blockedAdmissions"`
	FailedWrites            int64   `json:"failedWrites"`
	DeadLetters             int64   `json:"deadLetters"`
	Dropped                 int64   `json:"dropped"`
	Pending                 int64   `json:"pending"`
	OldestPendingAgeSeconds float64 `json:"oldestPendingAgeSeconds"`
}
type persistReq struct {
	event    Event
	reserved int64
	done     chan error
}

type Durable struct {
	Pool                  *pgxpool.Pool
	Log                   *slog.Logger
	write                 func(context.Context, Event) error
	blocked, failed, dead atomic.Int64
	batchMu               sync.Mutex
	flushing              bool
	pendingBatch          []*persistReq
	queuedBytes           atomic.Int64
}

func NewDurable(pool *pgxpool.Pool, log *slog.Logger) *Durable { return &Durable{Pool: pool, Log: log} }
func (d *Durable) Stats() Counters {
	return Counters{Mode: "strict", PendingBytes: d.queuedBytes.Load(), BlockedAdmissions: d.blocked.Load(), FailedWrites: d.failed.Load(), DeadLetters: d.dead.Load()}
}

func (d *Durable) groupPersist(ctx context.Context, e Event) error {
	if len(batchFromTest(d)) == 0 && d.Pool == nil {
		return Persist(ctx, d.Pool, e)
	}
	size := int64(len(e.Ciphertext) + 4096)
	if d.queuedBytes.Add(size) > 32<<20 {
		d.queuedBytes.Add(-size)
		return errors.New("strict audit queue capacity exceeded")
	}
	e.RawPayload = nil
	e.Policy = nil
	req := &persistReq{event: e, done: make(chan error, 1), reserved: size}
	d.batchMu.Lock()
	d.pendingBatch = append(d.pendingBatch, req)
	if d.flushing {
		d.batchMu.Unlock()
		select {
		case err := <-req.done:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	d.flushing = true
	d.batchMu.Unlock()

	go d.flushPendingBatches()

	select {
	case err := <-req.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func batchFromTest(*Durable) []*persistReq { return nil }

func (d *Durable) flushPendingBatches() {
	for {
		runtime.Gosched()
		d.batchMu.Lock()
		if len(d.pendingBatch) == 0 {
			d.flushing = false
			d.batchMu.Unlock()
			return
		}
		n := min(len(d.pendingBatch), 64)
		batch := make([]*persistReq, n)
		copy(batch, d.pendingBatch[:n])
		d.pendingBatch = d.pendingBatch[n:]
		d.batchMu.Unlock()

		if len(batch) == 1 {
			bctx, cancel := context.WithTimeout(context.Background(), WriteTimeout)
			err := Persist(bctx, d.Pool, batch[0].event)
			cancel()
			d.queuedBytes.Add(-batch[0].reserved)
			batch[0].done <- err
			continue
		}

		bctx, cancel := context.WithTimeout(context.Background(), WriteTimeout)
		err := persistBatch(bctx, d.Pool, batch)
		cancel()
		if err == nil {
			for _, r := range batch {
				d.queuedBytes.Add(-r.reserved)
				r.done <- nil
			}
			continue
		}
		// Fall back to individual persistence so a single bad event cannot fail valid peers.
		for _, r := range batch {
			ictx, icancel := context.WithTimeout(context.Background(), WriteTimeout)
			persistErr := Persist(ictx, d.Pool, r.event)
			d.queuedBytes.Add(-r.reserved)
			r.done <- persistErr
			icancel()
		}
	}
}

func persistBatch(ctx context.Context, pool *pgxpool.Pool, batch []*persistReq) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `SET LOCAL synchronous_commit=on`); err != nil {
		return err
	}
	var fallbackRev int64
	statements := &pgx.Batch{}
	for _, r := range batch {
		e := r.event
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
		statements.Queue(insertSQL, args...)
		if len(e.Ciphertext) > 0 {
			statements.Queue(`INSERT INTO audit_payloads(event_id,ciphertext,expires_at) VALUES($1::uuid,$2,$3) ON CONFLICT(event_id) DO NOTHING`, e.EventID, e.Ciphertext, e.PayloadExpiresAt)
		}
	}
	results := tx.SendBatch(ctx, statements)
	for range statements.Len() {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			return err
		}
	}
	if err := results.Close(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (d *Durable) persist(ctx context.Context, e Event) error {
	ctx, cancel := context.WithTimeout(ctx, WriteTimeout)
	defer cancel()
	write := d.write
	if write == nil {
		write = d.groupPersist
	}
	delay := 25 * time.Millisecond
	for {
		err := write(ctx, e)
		if err == nil {
			return nil
		}
		d.failed.Add(1)
		if !transient(err) {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				d.dead.Add(1)
			}
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay = min(delay*2, 250*time.Millisecond)
	}
}
func (d *Durable) Admit(ctx context.Context, e Event) (Event, error) {
	e.Stage = "admission"
	e.Status = "started"
	var err error
	if e.OperationID == "" {
		e.OperationID, err = NewID()
	}
	if e.RequestID == "" {
		e.RequestID = e.OperationID
	}
	if err == nil {
		e, err = Prepare(e)
	}
	if err == nil {
		err = d.persist(ctx, e)
	}
	if err != nil {
		d.blocked.Add(1)
		if d.Log != nil {
			d.Log.Error("audit admission blocked", "operation_id", e.OperationID)
		}
	}
	return e, err
}
func (d *Durable) Complete(ctx context.Context, e Event) error {
	if e.OperationID == "" {
		return errors.New("audit outcome requires an admission")
	}
	e.Stage = "outcome"
	// Stable outcome identity makes retries across recorder/process instances idempotent.
	e.EventID = OutcomeID(e.OperationID)
	e.Time = time.Now().UTC()
	e, err := Prepare(e)
	if err != nil {
		return err
	}
	return d.persist(ctx, e)
}

// Record is for standalone denials. Protected actions use Admit and Complete instead.
func (d *Durable) Record(e Event) {
	e, err := Prepare(e)
	if err == nil {
		err = d.persist(context.Background(), e)
	}
	if err != nil && d.Log != nil {
		d.Log.Error("audit event unavailable", "event_id", e.EventID)
	}
}

type PendingStatus struct {
	Incomplete                 int64   `json:"incomplete"`
	OldestIncompleteAgeSeconds float64 `json:"oldestIncompleteAgeSeconds"`
}

func Pending(ctx context.Context, pool *pgxpool.Pool) (PendingStatus, error) {
	var status PendingStatus
	ctx, cancel := context.WithTimeout(ctx, WriteTimeout)
	defer cancel()
	err := pool.QueryRow(ctx, `SELECT count(*),COALESCE(EXTRACT(EPOCH FROM now()-min(a.ts)),0)::float8 FROM audit_events a WHERE a.stage='admission' AND NOT EXISTS(SELECT 1 FROM audit_events o WHERE o.operation_id=a.operation_id AND o.stage='outcome')`).Scan(&status.Incomplete, &status.OldestIncompleteAgeSeconds)
	return status, err
}

// Transaction metadata follows the authorized HTTP request into Store.Do.
type transactionKey struct{}

func WithTransaction(ctx context.Context, e Event) context.Context {
	return context.WithValue(ctx, transactionKey{}, e)
}
func Transaction(ctx context.Context) (Event, bool) {
	e, ok := ctx.Value(transactionKey{}).(Event)
	return e, ok
}

// Service supplies uniform lifecycle and diagnostic hooks for both modes.
type Service interface {
	Recorder
	Stats() Counters
	Done() <-chan struct{}
}

var synchronousDone = func() <-chan struct{} { ch := make(chan struct{}); close(ch); return ch }()

func (d *Durable) Done() <-chan struct{} { return synchronousDone }
func NewService(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger, mode string) (Service, error) {
	switch mode {
	case "strict":
		return NewDurable(pool, log), nil
	case "best_effort":
		return NewWriter(ctx, pool, log), nil
	default:
		return nil, errors.New("VMCP_AUDIT_MODE must be strict or best_effort")
	}
}

type traceKey struct{}

func WithTrace(ctx context.Context, parent string) context.Context {
	return context.WithValue(ctx, traceKey{}, TraceID(parent))
}
func TraceFrom(ctx context.Context) string { v, _ := ctx.Value(traceKey{}).(string); return v }

func ValidID(id string) bool { return uuidPattern.MatchString(id) }

// WithFailureReporter exposes transaction-local audit failures to the owning process.
type failureReporterKey struct{}

func WithFailureReporter(ctx context.Context, report func(error)) context.Context {
	return context.WithValue(ctx, failureReporterKey{}, report)
}
func ReportTransactionFailure(ctx context.Context, err error) {
	if report, ok := ctx.Value(failureReporterKey{}).(func(error)); ok {
		report(err)
	}
}
func (d *Durable) ReportWriteFailure(err error) {
	d.failed.Add(1)
	if !transient(err) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		d.dead.Add(1)
	}
}

func OutcomeID(operation string) string {
	digest := sha256.Sum256([]byte("vmcp.audit.outcome:" + operation))
	digest[6] = (digest[6] & 15) | 128
	digest[8] = (digest[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", digest[:4], digest[4:6], digest[6:8], digest[8:10], digest[10:16])
}
