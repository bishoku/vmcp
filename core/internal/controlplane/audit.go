package controlplane

import (
	"net/http"
	"strconv"
	"time"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/store"
)

func (s *Server) listAuditEvents(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	var errs []apierror.FieldError
	limit := defaultLimit
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxLimit {
			errs = append(errs, apierror.FieldError{Field: "limit", Code: apierror.CodeValidationFailed, Message: "must be between 1 and 200"})
		} else {
			limit = n
		}
	}
	f := store.AuditFilter{VirtualServer: q.Get("virtualServer"), AgentKey: q.Get("agentKey"), Subject: q.Get("subject"), Tool: q.Get("tool"), Status: q.Get("status")}
	if f.Status != "" && f.Status != "started" && f.Status != "ok" && f.Status != "tool_error" && f.Status != "error" && f.Status != "denied" {
		errs = append(errs, apierror.FieldError{Field: "status", Code: apierror.CodeValidationFailed, Message: "must be one of: started, ok, tool_error, error, denied"})
	}
	f.Stage = q.Get("stage")
	f.OperationID = q.Get("operationId")
	f.RequestID = q.Get("requestId")
	f.ActorID = q.Get("actorId")
	if f.Stage != "" && f.Stage != "event" && f.Stage != "admission" && f.Stage != "outcome" && f.Stage != "mutation" {
		errs = append(errs, apierror.FieldError{Field: "stage", Code: apierror.CodeValidationFailed, Message: "invalid stage"})
	}
	if v := q.Get("incomplete"); v != "" {
		var err error
		f.Incomplete, err = strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, apierror.FieldError{Field: "incomplete", Code: apierror.CodeValidationFailed, Message: "must be boolean"})
		}
	}
	for field, id := range map[string]string{"operationId": f.OperationID, "requestId": f.RequestID} {
		if id != "" && !audit.ValidID(id) {
			errs = append(errs, apierror.FieldError{Field: field, Code: apierror.CodeValidationFailed, Message: "must be UUID"})
		}
	}
	for name, dst := range map[string]**time.Time{"from": &f.From, "to": &f.To} {
		if v := q.Get(name); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				errs = append(errs, apierror.FieldError{Field: name, Code: apierror.CodeValidationFailed, Message: "must be an RFC 3339 timestamp"})
				continue
			}
			*dst = &t
		}
	}
	if v := q.Get("cursor"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id < 1 {
			errs = append(errs, apierror.FieldError{Field: "cursor", Code: apierror.CodeValidationFailed, Message: "invalid cursor"})
		}
		f.BeforeID = id
	}
	if len(errs) > 0 {
		return apierror.Validation(errs)
	}
	items, more, err := s.Store.Reader().ListAudit(r.Context(), f, limit)
	if err != nil {
		return err
	}
	var next *string
	if more {
		c := strconv.FormatInt(items[len(items)-1].ID, 10)
		next = &c
	}
	return writeJSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": next})
}
