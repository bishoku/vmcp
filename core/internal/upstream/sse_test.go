package upstream

import (
	"context"
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/tlsconf"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	fx "github.com/bishoku/vmcp/core/internal/fixture"
)

func TestSSEClientAgainstLegacyFixture(t *testing.T) {
	srv := httptest.NewServer(fx.NewLegacySSE(fx.Options{Token: "s3"}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Wrong token is rejected.
	if _, err := NewSSEClient(srv.URL, nil, nil).Initialize(ctx); err == nil {
		t.Fatal("expected auth failure")
	}

	c, err := Open("sse", srv.URL, map[string]string{"Authorization": "Bearer s3"}, tlsconf.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	init, err := c.Initialize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(init.ServerInfo.Name, "legacy") {
		t.Fatalf("server=%q", init.ServerInfo.Name)
	}
	tools, err := c.ListTools(ctx)
	if err != nil || len(tools) == 0 {
		t.Fatalf("tools=%v err=%v", tools, err)
	}
	raw, err := c.CallTool(ctx, "add", json.RawMessage(`{"a":2,"b":40}`))
	if err != nil || !strings.Contains(string(raw), "42") {
		t.Fatalf("raw=%s err=%v", raw, err)
	}

	// Stream closure surfaces as an expired session so callers can reconnect.
	_ = c.Close(ctx)
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, err = c.CallTool(ctx, "add", json.RawMessage(`{"a":1,"b":1}`))
		if err == ErrSessionExpired {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("err=%v, want ErrSessionExpired", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
