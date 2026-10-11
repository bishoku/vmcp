package resilience

import (
	"context"
	"errors"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/oauthtoken"
	"testing"
)

func TestSharedErrorClassification(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tt := range []struct {
		ctx  context.Context
		err  error
		want Outcome
	}{
		{context.Background(), &oauthtoken.Error{Err: errors.New("IdP unavailable")}, Ignore},
		{ctx, errors.New("cancelled transport"), Ignore},
		{context.Background(), &mcp.Error{Code: -1, Message: "application"}, Reached},
		{context.Background(), errors.New("connection refused"), Failed},
	} {
		if got := Classify(tt.ctx, tt.err); got != tt.want {
			t.Fatalf("%v: got %v want %v", tt.err, got, tt.want)
		}
	}
}
