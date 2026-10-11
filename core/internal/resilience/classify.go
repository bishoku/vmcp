package resilience

import (
	"context"
	"errors"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/oauthtoken"
)

type Outcome int

const (
	Ignore Outcome = iota
	Reached
	Failed
)

// Classify keeps identity-provider and caller failures out of upstream health decisions.
func Classify(ctx context.Context, err error) Outcome {
	var rpc *mcp.Error
	if err == nil || errors.As(err, &rpc) {
		return Reached
	}
	if ctx.Err() != nil || oauthtoken.IsTokenError(err) {
		return Ignore
	}
	return Failed
}
