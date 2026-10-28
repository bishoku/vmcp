package store

import (
	"context"
	"errors"
	"strings"

	"github.com/bishoku/vmcp/core/internal/apierror"
)

const MaxBulkToolGrants = 200

type ToolGrantResult struct {
	VirtualServer string `json:"virtualServer"`
	Upstream      string `json:"upstream"`
	Tool          string `json:"tool"`
	Result        string `json:"result"`
	Code          string `json:"code,omitempty"`
	Message       string `json:"message,omitempty"`
}
type BulkToolGrantResult struct {
	Items     []ToolGrantResult `json:"items"`
	Changed   int               `json:"changed"`
	Unchanged int               `json:"unchanged"`
	Failed    int               `json:"failed"`
}

// BulkToolGrants deliberately commits each item independently, using the same
// revision lock, ownership/version checks and per-target audit as a single write.
// Clients must inspect every result and must not replay successful items.
func (s *Store) BulkToolGrants(ctx context.Context, items []ToolGrant) (BulkToolGrantResult, error) {
	out := BulkToolGrantResult{Items: []ToolGrantResult{}}
	if access, ok := AccessFrom(ctx); ok && (!access.Write || access.Role == "viewer") {
		return out, apierror.Forbidden("write access required")
	}
	if len(items) == 0 || len(items) > MaxBulkToolGrants {
		return out, apierror.Validation([]apierror.FieldError{vf("/items", "provide 1–200 tool grants")})
	}
	seen := map[[3]string]bool{}
	for _, g := range items {
		if (g.Status == "rejected" || g.Status == "revoked") && strings.TrimSpace(g.Reason) == "" {
			return out, apierror.Validation([]apierror.FieldError{vf("/items", "a reason is required for rejection or revocation")})
		}
		key := [3]string{g.VirtualServer, g.Upstream, g.Tool}
		if seen[key] {
			return out, apierror.Validation([]apierror.FieldError{vf("/items", "duplicate tool grant")})
		}
		seen[key] = true
	}
	for _, g := range items {
		changed := false
		err := s.Do(ctx, func(t *Tx) error { return t.putToolGrant(ctx, g, &changed) })
		item := ToolGrantResult{VirtualServer: g.VirtualServer, Upstream: g.Upstream, Tool: g.Tool, Result: "unchanged"}
		if err != nil {
			item.Result = "failed"
			item.Code, item.Message = "internal", "Update failed; refresh before retrying this item."
			var problem *apierror.Error
			if errors.As(err, &problem) {
				item.Code, item.Message = problem.Code, problem.Detail
			}
			out.Failed++
		} else if changed {
			item.Result = "changed"
			out.Changed++
		} else {
			out.Unchanged++
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}
