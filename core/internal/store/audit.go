package store

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// AuditEvent is the API read model of an audit row.
type AuditEvent struct {
	Mode                 string  `json:"auditMode"`
	PolicyRevision       int64   `json:"policyRevision"`
	Scope                string  `json:"scope"`
	Boundary             string  `json:"boundary"`
	Direction            string  `json:"direction"`
	ParentOperationID    *string `json:"parentOperationId"`
	PayloadState         string  `json:"payloadState"`
	PayloadOriginalBytes int     `json:"payloadOriginalBytes"`
	PayloadStoredBytes   int     `json:"payloadStoredBytes"`
	PayloadRedactions    int     `json:"payloadRedactions"`

	TenantID      string    `json:"tenantId"`
	EventID       string    `json:"eventId"`
	OperationID   *string   `json:"operationId"`
	RequestID     *string   `json:"requestId"`
	TraceID       string    `json:"traceId"`
	SchemaVersion int       `json:"schemaVersion"`
	ActorType     string    `json:"actorType"`
	ActorID       string    `json:"actorId"`
	Issuer        string    `json:"issuer"`
	Action        string    `json:"action"`
	Target        string    `json:"target"`
	Revision      int64     `json:"configRevision"`
	Stage         string    `json:"stage"`
	ID            int64     `json:"id"`
	Time          time.Time `json:"time"`
	VirtualServer string    `json:"virtualServer"`
	AgentKey      *string   `json:"agentKey"`
	ClientIP      *string   `json:"clientIp"`
	Method        string    `json:"method"`
	Subject       *string   `json:"subject"`
	Tool          *string   `json:"tool"`
	Upstream      *string   `json:"upstream"`
	Status        string    `json:"status"`
	Reason        *string   `json:"reason"`
	DurationMS    int       `json:"durationMs"`
	Redactions    int       `json:"redactions"`
}

// AuditFilter narrows ListAudit. Zero values mean "any".
type AuditFilter struct {
	VirtualServer, AgentKey, Subject, Tool, Status string
	Stage, OperationID, RequestID, ActorID         string
	Incomplete                                     bool
	From, To                                       *time.Time
	BeforeID                                       int64 // cursor: return events with a smaller id
}

// ListAudit returns events newest first.
func (t *Tx) ListAudit(ctx context.Context, f AuditFilter, limit int) ([]AuditEvent, bool, error) {
	var where []string
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "?", "$"+strconv.Itoa(len(args))))
	}
	if f.Incomplete {
		where = append(where, `stage='admission' AND NOT EXISTS(SELECT 1 FROM audit_events o WHERE o.operation_id=audit_events.operation_id AND o.stage='outcome')`)
	}
	for column, value := range map[string]string{"stage": f.Stage, "operation_id": f.OperationID, "request_id": f.RequestID, "actor_id": f.ActorID} {
		if value != "" {
			if column == "operation_id" {
				args = append(args, value)
				n := strconv.Itoa(len(args))
				where = append(where, "(operation_id=$"+n+"::uuid OR parent_operation_id=$"+n+"::uuid)")
			} else {
				add(column+" = ?", value)
			}
		}
	}
	if f.VirtualServer != "" {
		add("virtual_server = ?", f.VirtualServer)
	}
	if f.AgentKey != "" {
		add("agent_key = ?", f.AgentKey)
	}
	if f.Subject != "" {
		add("subject = ?", f.Subject)
	}
	if f.Tool != "" {
		add("tool = ?", f.Tool)
	}
	if f.Status != "" {
		add("status = ?", f.Status)
	}
	if f.From != nil {
		add("ts >= ?", *f.From)
	}
	if f.To != nil {
		add("ts < ?", *f.To)
	}
	if f.BeforeID > 0 {
		add("id < ?", f.BeforeID)
	}
	q := `SELECT tenant_id,event_id::text,operation_id::text,request_id::text,trace_id,schema_version,actor_type,actor_id,issuer,action,target,config_revision,stage,id, ts, virtual_server, agent_key, client_ip, method, subject, tool, upstream, status, reason, duration_ms, redactions,audit_mode,policy_revision,scope,boundary,direction,parent_operation_id::text,payload_state,payload_original_bytes,payload_stored_bytes,payload_redactions FROM audit_events`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	args = append(args, limit+1)
	q += " ORDER BY id DESC LIMIT $" + strconv.Itoa(len(args))
	rows, err := t.q.Query(ctx, q, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []AuditEvent{}
	for rows.Next() {
		var e AuditEvent
		if err := rows.Scan(&e.TenantID, &e.EventID, &e.OperationID, &e.RequestID, &e.TraceID, &e.SchemaVersion, &e.ActorType, &e.ActorID, &e.Issuer, &e.Action, &e.Target, &e.Revision, &e.Stage, &e.ID, &e.Time, &e.VirtualServer, &e.AgentKey, &e.ClientIP, &e.Method, &e.Subject, &e.Tool, &e.Upstream, &e.Status, &e.Reason, &e.DurationMS, &e.Redactions, &e.Mode, &e.PolicyRevision, &e.Scope, &e.Boundary, &e.Direction, &e.ParentOperationID, &e.PayloadState, &e.PayloadOriginalBytes, &e.PayloadStoredBytes, &e.PayloadRedactions); err != nil {
			return nil, false, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}
