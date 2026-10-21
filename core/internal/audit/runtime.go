package audit

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Runtime keeps a stable recorder while settings change. Each admitted operation
// retains its immutable settings; switching modes never rewrites an in-flight call.
var ErrUnavailable = errors.New("audit persistence unavailable")

type Runtime struct {
	Strict   *Durable
	Relax    *Writer
	Box      *secrets.Box
	fallback string
	current  atomic.Pointer[Settings]
}

func NewRuntime(ctx context.Context, pool *pgxpool.Pool, box *secrets.Box, log *slog.Logger, fallback string) *Runtime {
	if fallback == "best_effort" {
		fallback = "relax"
	}
	if fallback != "relax" {
		fallback = "strict"
	}
	r := &Runtime{Strict: NewDurable(pool, log), Relax: NewWriter(ctx, pool, log), Box: box, fallback: fallback}
	s := DefaultSettings()
	r.current.Store(&s)
	return r
}
func (r *Runtime) SetSettings(s Settings) {
	s = s.Normalize()
	for {
		old := r.current.Load()
		if old.Revision > s.Revision {
			return
		}
		if r.current.CompareAndSwap(old, &s) {
			return
		}
	}
}
func (r *Runtime) Settings() Settings { return *r.current.Load() }
func (r *Runtime) Pin(e Event) Event {
	if e.Policy == nil {
		s := r.Settings()
		e.Policy = &s
	}
	normalized := e.Policy.Normalize()
	e.Policy = &normalized
	e.Mode = e.Policy.Mode
	if e.Mode == "" {
		e.Mode = r.fallback
	}
	e.PolicyRevision = e.Policy.Revision
	return e
}
func (r *Runtime) prepare(e Event) (Event, error) {
	e = r.Pin(e)
	var err error
	e, err = Prepare(e)
	if err != nil {
		return e, err
	}
	if e.Direction == "" {
		e.PayloadState = "disabled"
		e.Ciphertext = nil
		return e, nil
	}
	p, err := PreviewPayload(*e.Policy, e.Scope, e.Direction, e.VirtualServer, e.Upstream, e.RawPayload)
	// Never retain caller-owned buffers or raw values in an asynchronous queue.
	e.RawPayload = nil
	e.Ciphertext = nil
	if err != nil {
		return e, err
	}
	e.PayloadState = p.State
	e.PayloadOriginalBytes = max(p.OriginalBytes, e.RawPayloadBytes)
	e.RawPayloadBytes = 0
	e.PayloadStoredBytes = p.StoredBytes
	e.PayloadRedactions = p.Redactions
	if len(p.Content) > 0 {
		if r.Box == nil {
			return e, errors.New("audit encryption unavailable")
		}
		e.Ciphertext, err = r.Box.SealBound(p.Content, e.EventID, "audit.payload")
		e.PayloadExpiresAt = time.Now().UTC().Add(time.Duration(e.Policy.RetentionDays) * 24 * time.Hour)
	}
	return e, err
}
func (r *Runtime) Admit(ctx context.Context, e Event) (Event, error) {
	e = r.Pin(e)
	if e.OperationID == "" {
		id, err := NewID()
		if err != nil {
			return e, err
		}
		e.OperationID = id
	}
	if e.RequestID == "" {
		e.RequestID = e.OperationID
	}
	e.Stage = "admission"
	e.Status = "started"
	e.Direction = "request"
	prepared, err := r.prepare(e)
	if err != nil {
		return e, r.failure(e, err)
	}
	if prepared.Mode == "relax" {
		r.Relax.Record(prepared)
		return prepared, nil
	}
	admitted, err := r.Strict.Admit(ctx, prepared)
	if err != nil {
		return admitted, ErrUnavailable
	}
	return admitted, nil
}
func (r *Runtime) Complete(ctx context.Context, e Event) error {
	e = r.Pin(e)
	e.Stage = "outcome"
	e.EventID = OutcomeID(e.OperationID)
	e.Time = time.Now().UTC()
	e.Direction = "response"
	p, err := r.prepare(e)
	if err != nil {
		return r.failure(e, err)
	}
	if p.Mode == "relax" {
		r.Relax.Record(p)
		return nil
	}
	if err := r.Strict.Complete(ctx, p); err != nil {
		return ErrUnavailable
	}
	return nil
}
func (r *Runtime) Record(e Event) {
	e = r.Pin(e)
	p, err := r.prepare(e)
	if err != nil {
		_ = r.failure(e, err)
		return
	}
	if p.Mode == "relax" {
		r.Relax.Record(p)
	} else {
		r.Strict.Record(p)
	}
}
func (r *Runtime) failure(e Event, err error) error {
	if e.Mode == "relax" {
		r.Relax.dead.Add(1)
		r.Relax.dropped.Add(1)
		return nil
	}
	r.Strict.ReportWriteFailure(err)
	return ErrUnavailable
}
func (r *Runtime) Stats() Counters {
	s := r.Strict.Stats()
	b := r.Relax.Stats()
	settings := r.Settings()
	s.Mode = settings.Mode
	if s.Mode == "" {
		s.Mode = r.fallback
	}
	s.PolicyRevision = settings.Revision
	s.PendingBytes += r.Relax.pendingBytes.Load()
	s.Dropped = b.Dropped
	s.FailedWrites += b.FailedWrites
	s.DeadLetters += b.DeadLetters
	s.Pending = b.Pending
	s.OldestPendingAgeSeconds = b.OldestPendingAgeSeconds
	return s
}
func (r *Runtime) Done() <-chan struct{}        { return r.Relax.Done() }
func (r *Runtime) ReportWriteFailure(err error) { r.Strict.ReportWriteFailure(err) }

// Mutation is called before committing in strict mode, after committing in relax.
func (r *Runtime) Mutation(ctx context.Context, e Event, tx Sink) error {
	e.EventID = ""
	e.Stage = "mutation"
	e.Status = "ok"
	e.Direction = ""
	e.RawPayload = nil
	e.Ciphertext = nil
	p, err := r.prepare(e)
	if err != nil {
		return r.failure(e, err)
	}
	if p.Mode == "relax" {
		r.Relax.Record(p)
		return nil
	}
	return Insert(ctx, tx, p)
}

type operationContext struct {
	runtime *Runtime
	base    Event
}
type operationKey struct{}

func WithOperation(ctx context.Context, r *Runtime, e Event) context.Context {
	return context.WithValue(ctx, operationKey{}, operationContext{r, r.Pin(e)})
}
func InheritOperation(ctx context.Context, e Event) Event {
	if op, ok := ctx.Value(operationKey{}).(operationContext); ok {
		e.Policy = op.base.Policy
		e.Mode = op.base.Mode
		e.PolicyRevision = op.base.PolicyRevision
	}
	return e
}
func RuntimeFrom(ctx context.Context) *Runtime {
	op, _ := ctx.Value(operationKey{}).(operationContext)
	return op.runtime
}

// Observe records a protocol request, including each actual retry, with its own
// lifecycle. Raw credentials/headers are never part of these protocol payloads.
func Observe(ctx context.Context, method string, params any) (func(json.RawMessage, error) error, error) {
	op, ok := ctx.Value(operationKey{}).(operationContext)
	if !ok || op.runtime == nil {
		return func(json.RawMessage, error) error { return nil }, nil
	}
	captureScope := "upstream"
	if op.base.Scope == "system" {
		captureScope = "system"
	}
	capture := op.base.Policy.Scopes[captureScope]
	if !capture.Request && !capture.Response {
		return func(json.RawMessage, error) error { return nil }, nil
	}
	e := op.base
	e.ParentOperationID = e.OperationID
	e.OperationID = ""
	e.EventID = ""
	e.Scope = "upstream"
	if op.base.Scope == "system" {
		e.Scope = "system"
	}
	e.Boundary = "upstream"
	e.Action = method
	e.Method = method
	e.Time = time.Now().UTC()
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, errors.New("audit request serialization failed")
	}
	e.RawPayload = raw
	e, err = op.runtime.Admit(ctx, e)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	return func(result json.RawMessage, callErr error) error {
		e.Status = "ok"
		e.RawPayload = result
		var resultState struct {
			IsError    bool `json:"isError"`
			StatusCode int  `json:"statusCode"`
		}
		if json.Unmarshal(result, &resultState) == nil {
			if resultState.IsError {
				e.Status = "tool_error"
			}
			if resultState.StatusCode >= 400 {
				e.Status = "error"
			}
		}
		if callErr != nil {
			e.Status = "error"
			var rpcErr *mcp.Error
			if errors.As(callErr, &rpcErr) {
				e.RawPayload, _ = json.Marshal(map[string]any{"error": rpcErr})
			} else {
				e.RawPayload, _ = json.Marshal(map[string]string{"error": "upstream operation failed"})
			}
		}
		e.DurationMS = int(time.Since(start).Milliseconds())
		completion, cancel := context.WithTimeout(context.WithoutCancel(ctx), WriteTimeout)
		defer cancel()
		return op.runtime.Complete(completion, e)
	}, nil
}
func WithUpstream(ctx context.Context, slug string) context.Context {
	if op, ok := ctx.Value(operationKey{}).(operationContext); ok {
		op.base.Upstream = slug
		return context.WithValue(ctx, operationKey{}, op)
	}
	return ctx
}

// Write acknowledges a standalone fact in strict mode and only offers it to the
// bounded queue in relax mode. Unlike Record, callers can enforce its result.
func (r *Runtime) Write(ctx context.Context, e Event) error {
	e = r.Pin(e)
	p, err := r.prepare(e)
	if err != nil {
		return r.failure(e, err)
	}
	if p.Mode == "relax" {
		r.Relax.Record(p)
		return nil
	}
	if err := Persist(ctx, r.Strict.Pool, p); err != nil {
		r.Strict.ReportWriteFailure(err)
		return ErrUnavailable
	}
	return nil
}

// ObservationEnabled lets protocol adapters avoid copying or serializing bodies
// when optional upstream/system capture is disabled.
func ObservationEnabled(ctx context.Context) bool {
	op, ok := ctx.Value(operationKey{}).(operationContext)
	if !ok || op.runtime == nil || op.base.Policy == nil {
		return false
	}
	scope := "upstream"
	if op.base.Scope == "system" {
		scope = "system"
	}
	c := op.base.Policy.Scopes[scope]
	return c.Request || c.Response
}
