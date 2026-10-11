package audit

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

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

func Insert(ctx context.Context, q Sink, e Event) error {
	e, err := Prepare(e)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `INSERT INTO audit_events
 (event_id,operation_id,request_id,trace_id,schema_version,actor_type,actor_id,issuer,action,target,config_revision,stage,
 ts,virtual_server,agent_key,client_ip,method,subject,tool,upstream,status,reason,duration_ms,redactions)
 VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)
 ON CONFLICT(event_id) DO NOTHING`, e.EventID, nullable(e.OperationID), nullable(e.RequestID), e.TraceID, e.SchemaVersion, e.ActorType, e.ActorID, e.Issuer, e.Action, e.Target, e.Revision, e.Stage, e.Time, e.VirtualServer, nullable(e.AgentKey), nullable(e.ClientIP), e.Method, nullable(e.Subject), nullable(e.Tool), nullable(e.Upstream), e.Status, nullable(e.Reason), e.DurationMS, e.Redactions)
	return err
}

// Persist acknowledges local PostgreSQL WAL durability, independent of session defaults.
func Persist(ctx context.Context, pool *pgxpool.Pool, e Event) error {
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
	Mode                    string  `json:"mode"`
	BlockedAdmissions       int64   `json:"blockedAdmissions"`
	FailedWrites            int64   `json:"failedWrites"`
	DeadLetters             int64   `json:"deadLetters"`
	Dropped                 int64   `json:"dropped"`
	Pending                 int64   `json:"pending"`
	OldestPendingAgeSeconds float64 `json:"oldestPendingAgeSeconds"`
}
type Durable struct {
	Pool                  *pgxpool.Pool
	Log                   *slog.Logger
	write                 func(context.Context, Event) error
	blocked, failed, dead atomic.Int64
}

func NewDurable(pool *pgxpool.Pool, log *slog.Logger) *Durable { return &Durable{Pool: pool, Log: log} }
func (d *Durable) Stats() Counters {
	return Counters{Mode: "strict", BlockedAdmissions: d.blocked.Load(), FailedWrites: d.failed.Load(), DeadLetters: d.dead.Load()}
}
func (d *Durable) persist(ctx context.Context, e Event) error {
	ctx, cancel := context.WithTimeout(ctx, WriteTimeout)
	defer cancel()
	write := d.write
	if write == nil {
		write = func(ctx context.Context, e Event) error { return Persist(ctx, d.Pool, e) }
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
	digest := sha256.Sum256([]byte("vmcp.audit.outcome:" + e.OperationID))
	digest[6] = (digest[6] & 15) | 128
	digest[8] = (digest[8] & 63) | 128
	e.EventID = fmt.Sprintf("%x-%x-%x-%x-%x", digest[:4], digest[4:6], digest[6:8], digest[8:10], digest[10:16])
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
