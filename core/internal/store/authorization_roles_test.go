package store

import (
	"context"
	"errors"
	"testing"

	"github.com/bishoku/vmcp/core/internal/apierror"
)

// Authorization management must not inherit the implicit worker bypass used by
// ordinary configuration stores. These calls must reject before touching SQL.
func TestAuthorizationRoleRequiresExplicitAdminContext(t *testing.T) {
	for _, ctx := range []context.Context{context.Background(), WithAccess(context.Background(), Access{Role: "viewer"}), WithAccess(context.Background(), Access{Role: "editor"}), WithAccess(context.Background(), Access{Role: "team", Groups: []string{"admins"}})} {
		tx := &Tx{}
		_, readErr := tx.AuthorizationRole(ctx, "observer")
		_, writeErr := tx.SaveAuthorizationRole(ctx, "", RoleInput{}, nil)
		deleteErr := tx.DeleteAuthorizationRole(ctx, "observer", 1)
		_, assignmentReadErr := tx.AuthorizationAssignment(ctx, "bad")
		_, assignmentWriteErr := tx.SaveAuthorizationAssignment(ctx, "", AssignmentInput{}, nil, "")
		assignmentDeleteErr := tx.DeleteAuthorizationAssignment(ctx, "bad", 1)
		_, _, scopeErr := tx.AuthorizationScopeOptions(ctx, "team", "", 1)
		_, _, epochErr := tx.AuthorizationResourceEpochs(ctx, "upstream", "bad", "", 1)
		_, mappingReadErr := tx.AuthorizationMapping(ctx, "bad")
		_, mappingWriteErr := tx.SaveAuthorizationMapping(ctx, "", MappingInput{}, nil, "")
		mappingDeleteErr := tx.DeleteAuthorizationMapping(ctx, "bad", 1)
		inspectionStore := &Store{}
		_, evaluateErr := inspectionStore.EvaluateAuthorization(ctx, AuthorizationEvaluationInput{}, "")
		_, statusErr := inspectionStore.AuthorizationStatus(ctx, "")
		for _, err := range []error{mappingReadErr, mappingWriteErr, mappingDeleteErr, evaluateErr, statusErr, readErr, writeErr, deleteErr, assignmentReadErr, assignmentWriteErr, assignmentDeleteErr, scopeErr, epochErr} {
			var e *apierror.Error
			if !errors.As(err, &e) || e.Status != 403 {
				t.Fatalf("expected explicit admin requirement, got %v", err)
			}
		}
	}
}
