package e2e

import (
	"context"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/upstream"
	"strings"
	"testing"
	"time"
)

func TestOwnerGrantControlsExistingMCPSession(t *testing.T) {
	e := setup(t)
	st := store.Store{Pool: e.pool, Box: e.syncer.Box}
	admin := store.WithAccess(e.ctx, store.Access{Role: "admin", Write: true, Actor: "platform"})
	mutate := func(fn func(*store.Tx) error) {
		t.Helper()
		if err := st.Do(admin, fn); err != nil {
			t.Fatal(err)
		}
	}
	mutate(func(tx *store.Tx) error {
		return tx.PutTeam(admin, store.Team{Slug: "crm", Name: "CRM", Issuer: "https://idp", Group: "crm"})
	})
	owner := "crm"
	mutate(func(tx *store.Tx) error {
		_, err := tx.PatchUpstream(admin, "alpha", store.UpstreamPatch{OwnerTeam: &owner}, nil)
		return err
	})
	wait := func(allowed bool) {
		t.Helper()
		for end := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
			_, ok := e.syncer.Snapshot().VirtualServers["agent"].Lookup("say")
			if ok == allowed {
				return
			}
			if time.Now().After(end) {
				t.Fatal("grant snapshot did not converge")
			}
		}
	}
	wait(false)
	c := upstream.NewClient(e.gwURL, nil, nil)
	if _, err := c.Initialize(e.ctx); err != nil {
		t.Fatal(err)
	}
	defer c.Close(context.Background())
	call := func(allowed bool) {
		t.Helper()
		raw, err := c.Call(e.ctx, "tools/list", nil)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), `"name":"say"`) != allowed {
			t.Fatalf("list visibility: %s", raw)
		}
		_, err = c.Call(e.ctx, "tools/call", map[string]any{"name": "say", "arguments": map[string]any{"text": "hello"}})
		if (err == nil) != allowed {
			t.Fatalf("call allowed=%v: %v", allowed, err)
		}
	}
	call(false)
	grant := store.ToolGrant{VirtualServer: "agent", Upstream: "alpha", Tool: "echo", Status: "approved"}
	mutate(func(tx *store.Tx) error { return tx.PutToolGrant(admin, grant) })
	wait(true)
	call(true)
	grant.Status = "revoked"
	grant.Version = 1
	mutate(func(tx *store.Tx) error { return tx.PutToolGrant(admin, grant) })
	wait(false)
	call(false)
}
