// Package apierror defines the API's error type and its RFC 9457 problem+json encoding.
package apierror

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
)

// Stable machine-readable codes (see the OpenAPI Problem schema).
const (
	CodeUnauthorized         = "unauthorized"
	CodeForbidden            = "forbidden"
	CodeNotFound             = "not_found"
	CodeAlreadyExists        = "already_exists"
	CodeInUse                = "in_use"
	CodePreconditionFailed   = "precondition_failed"
	CodeValidationFailed     = "validation_failed"
	CodeUnknownUpstream      = "unknown_upstream"
	CodeUnknownTool          = "unknown_tool"
	CodeUnknownVirtualServer = "unknown_virtual_server"
	CodeDuplicateExposed     = "duplicate_exposed_name"
	CodeUpstreamUnreach      = "upstream_unreachable"
	CodeInternal             = "internal"
)

// FieldError pinpoints one problem in a request. Field is a JSON pointer.
type FieldError struct {
	Field   string `json:"field,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

// Error is an error that maps to an HTTP problem response.
type Error struct {
	Status int
	Code   string
	Title  string
	Detail string
	Errors []FieldError
}

func (e *Error) Error() string { return e.Code + ": " + e.Detail }

func New(status int, code, title, detail string) *Error {
	return &Error{Status: status, Code: code, Title: title, Detail: detail}
}

func Unauthorized() *Error {
	return New(http.StatusUnauthorized, CodeUnauthorized, "Unauthorized", "missing or invalid bearer token")
}

func Forbidden(detail string) *Error {
	return New(http.StatusForbidden, CodeForbidden, "Forbidden", detail)
}

func NotFound(kind, slug string) *Error {
	return New(http.StatusNotFound, CodeNotFound, "Not found", fmt.Sprintf("%s %q does not exist", kind, slug))
}

func AlreadyExists(kind, slug string) *Error {
	return New(http.StatusConflict, CodeAlreadyExists, "Already exists", fmt.Sprintf("%s %q already exists", kind, slug))
}

func InUse(detail string) *Error {
	return New(http.StatusConflict, CodeInUse, "Resource in use", detail)
}

func PreconditionFailed() *Error {
	return New(http.StatusPreconditionFailed, CodePreconditionFailed, "Precondition failed",
		"If-Match does not match the current ETag; re-read the resource and retry")
}

func Internal() *Error {
	return New(http.StatusInternalServerError, CodeInternal, "Internal error", "an internal error occurred")
}

// Validation reports every problem found, not just the first.
func Validation(errs []FieldError) *Error {
	e := New(http.StatusUnprocessableEntity, CodeValidationFailed, "Validation failed",
		fmt.Sprintf("%d problem(s) found in the request", len(errs)))
	e.Errors = errs
	// A single, more specific code is more useful than the generic one.
	if len(errs) > 0 {
		same := true
		for _, f := range errs {
			if f.Code != errs[0].Code {
				same = false
			}
		}
		if same && errs[0].Code != "" && errs[0].Code != CodeValidationFailed {
			e.Code = errs[0].Code
		}
	}
	return e
}

// PrefixFields rewrites the field pointers of a validation error, used when a
// resource is validated as part of a larger document (apply).
func PrefixFields(err error, prefix string) error {
	var e *Error
	if !errors.As(err, &e) || len(e.Errors) == 0 {
		return err
	}
	c := *e
	c.Errors = make([]FieldError, len(e.Errors))
	for i, f := range e.Errors {
		f.Field = prefix + f.Field
		c.Errors[i] = f
	}
	return &c
}

type problem struct {
	Type     string       `json:"type"`
	Title    string       `json:"title"`
	Status   int          `json:"status"`
	Detail   string       `json:"detail,omitempty"`
	Instance string       `json:"instance,omitempty"`
	Code     string       `json:"code"`
	Errors   []FieldError `json:"errors,omitempty"`
}

// Write encodes err as application/problem+json. Unknown errors become a
// generic 500 and are logged, never leaked.
func Write(w http.ResponseWriter, r *http.Request, err error, log *slog.Logger) {
	var e *Error
	if !errors.As(err, &e) {
		log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		e = Internal()
	}
	if e.Status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(e.Status)
	_ = json.NewEncoder(w).Encode(problem{
		Type: "about:blank", Title: e.Title, Status: e.Status, Detail: e.Detail,
		Instance: r.URL.Path, Code: e.Code, Errors: e.Errors,
	})
}
