package store

import (
	"context"
	"github.com/bishoku/vmcp/core/internal/correlation"
	"strconv"
	"strings"
	"time"
)

// AuditEvent is the API read model of an audit row.
type AuditEvent struct {
	correlation.IDs
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
	ConversationID, RootConversationID, TurnID, ConversationGroupID string
	VirtualServer, AgentKey, Subject, Tool, Status                  string
	Stage, OperationID, RequestID, ActorID                          string
	Activity                                                        string
	Incomplete                                                      bool
	From, To                                                        *time.Time
	BeforeID                                                        int64 // cursor: return events with a smaller id
}

// ListAudit returns events newest first.
func (t *Tx) ListAudit(ctx context.Context, f AuditFilter, limit int) ([]AuditEvent, bool, error) {
	where, args := auditWhere(ctx, f, false)
	q := `SELECT tenant_id,event_id::text,operation_id::text,request_id::text,trace_id,schema_version,actor_type,actor_id,issuer,action,target,config_revision,stage,id, ts, virtual_server, agent_key, client_ip, method, subject, tool, upstream, status, reason, duration_ms, redactions,audit_mode,policy_revision,scope,boundary,direction,parent_operation_id::text,payload_state,payload_original_bytes,payload_stored_bytes,payload_redactions,conversation_id,root_conversation_id,turn_id,conversation_group_id,correlation_status FROM audit_events`
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
		if err := rows.Scan(&e.TenantID, &e.EventID, &e.OperationID, &e.RequestID, &e.TraceID, &e.SchemaVersion, &e.ActorType, &e.ActorID, &e.Issuer, &e.Action, &e.Target, &e.Revision, &e.Stage, &e.ID, &e.Time, &e.VirtualServer, &e.AgentKey, &e.ClientIP, &e.Method, &e.Subject, &e.Tool, &e.Upstream, &e.Status, &e.Reason, &e.DurationMS, &e.Redactions, &e.Mode, &e.PolicyRevision, &e.Scope, &e.Boundary, &e.Direction, &e.ParentOperationID, &e.PayloadState, &e.PayloadOriginalBytes, &e.PayloadStoredBytes, &e.PayloadRedactions, &e.ConversationID, &e.RootConversationID, &e.TurnID, &e.ConversationGroupID, &e.CorrelationStatus); err != nil {
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

func auditWhere(ctx context.Context, f AuditFilter, grouped bool) ([]string, []any) {
	var where []string
	if a, ok := AccessFrom(ctx); ok && a.Role != "admin" {
		where = append(where, `(EXISTS(SELECT 1 FROM virtual_servers v WHERE v.slug=audit_events.virtual_server AND `+vsPredicate(ctx, "v")+`) OR actor_id=`+literal(a.Actor)+`)`)
	}
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "?", "$"+strconv.Itoa(len(args))))
	}
	// Classify the whole call by its recorded origin. Upstream attempts inherit
	// membership through an explicit parent operation, never a shared trace ID
	// or virtual-server name (administrative routes can carry those too).
	switch f.Activity {
	case "virtual":
		where = append(where, `(scope='virtual' OR (scope='upstream' AND EXISTS(SELECT 1 FROM audit_events parent WHERE parent.operation_id=audit_events.parent_operation_id AND parent.scope='virtual')))`)
	case "control":
		where = append(where, `(scope IN ('control','reads') OR (scope='upstream' AND EXISTS(SELECT 1 FROM audit_events parent WHERE parent.operation_id=audit_events.parent_operation_id AND parent.scope IN ('control','reads'))))`)
	case "system":
		where = append(where, `scope='system'`)
	case "omni":
		where = append(where, `(scope IN ('omni','omni_diagnostics','omni_embedding','omni_rerank') OR EXISTS(SELECT 1 FROM audit_events parent WHERE parent.operation_id=audit_events.parent_operation_id AND (parent.scope='omni' OR (parent.scope='virtual' AND EXISTS(SELECT 1 FROM audit_events root WHERE root.operation_id=parent.parent_operation_id AND root.scope='omni')))))`)
	case "upstream":
		where = append(where, `scope='upstream'`)
	}
	if f.Incomplete {
		where = append(where, `stage='admission' AND NOT EXISTS(SELECT 1 FROM audit_events o WHERE o.operation_id=audit_events.operation_id AND o.stage='outcome')`)
	}
	for column, value := range map[string]string{"conversation_id": f.ConversationID, "root_conversation_id": f.RootConversationID, "turn_id": f.TurnID, "conversation_group_id": f.ConversationGroupID} {
		if value != "" {
			add(column+" = ?", value)
		}
	}
	for column, value := range map[string]string{"stage": f.Stage, "operation_id": f.OperationID, "request_id": f.RequestID, "actor_id": f.ActorID} {
		if value != "" {
			if column == "operation_id" {
				args = append(args, value)
				n := strconv.Itoa(len(args))
				where = append(where, "operation_id IN (WITH RECURSIVE linked(id) AS (SELECT $"+n+"::uuid UNION SELECT e.operation_id FROM audit_events e JOIN linked l ON e.parent_operation_id=l.id WHERE e.operation_id IS NOT NULL) SELECT id FROM linked)")
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
	if !grouped && f.BeforeID > 0 {
		add("id < ?", f.BeforeID)
	}

	return where, args
}

// Conversation summaries aggregate authorized, filtered events before pagination.
// Root operation counts exclude linked calls, provider events and diagnostics.
type AuditConversation struct {
	ConversationGroupID string    `json:"conversationGroupId"`
	RootConversationID  string    `json:"rootConversationId"`
	ActorID             string    `json:"actorId"`
	FirstTime           time.Time `json:"firstTime"`
	LastTime            time.Time `json:"lastTime"`
	LastID              int64     `json:"lastId"`
	Records             int64     `json:"records"`
	Operations          int64     `json:"operations"`
	Errors              int64     `json:"errors"`
	Conversations       int64     `json:"conversations"`
}

func (t *Tx) ListAuditConversations(ctx context.Context, f AuditFilter, limit int) ([]AuditConversation, bool, error) {
	where, args := auditWhere(ctx, f, true)
	where = append(where, "conversation_group_id<>''")
	q := `SELECT conversation_group_id,max(COALESCE(NULLIF(root_conversation_id,''),conversation_id)),max(actor_id),min(ts),max(ts),max(id),count(*),count(DISTINCT operation_id) FILTER(WHERE parent_operation_id IS NULL),count(DISTINCT operation_id) FILTER(WHERE parent_operation_id IS NULL AND stage='outcome' AND status IN ('error','tool_error','denied')),count(DISTINCT conversation_id) FILTER(WHERE conversation_id<>'') FROM audit_events WHERE ` + strings.Join(where, " AND ") + ` GROUP BY tenant_id,conversation_group_id`
	if f.BeforeID > 0 {
		args = append(args, f.BeforeID)
		q += " HAVING max(id)<$" + strconv.Itoa(len(args))
	}
	args = append(args, limit+1)
	q += " ORDER BY max(id) DESC LIMIT $" + strconv.Itoa(len(args))
	rows, err := t.q.Query(ctx, q, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []AuditConversation{}
	for rows.Next() {
		var v AuditConversation
		if err := rows.Scan(&v.ConversationGroupID, &v.RootConversationID, &v.ActorID, &v.FirstTime, &v.LastTime, &v.LastID, &v.Records, &v.Operations, &v.Errors, &v.Conversations); err != nil {
			return nil, false, err
		}
		out = append(out, v)
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
