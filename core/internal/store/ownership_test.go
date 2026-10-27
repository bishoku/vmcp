package store

import (
	"context"
	"encoding/base64"
	"github.com/bishoku/vmcp/core/internal/restapi"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/testdb"
	"testing"
)

func TestOwnershipIsolationApprovalAndRevocation(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	pool, err := Connect(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	box, _ := secrets.NewBox(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	st := Store{Pool: pool, Box: box}
	admin := WithAccess(ctx, Access{Role: "admin", Actor: "platform", Write: true})
	a := WithAccess(ctx, Access{Role: "team", Issuer: "https://idp", Groups: []string{"owners-a"}, Actor: "a", Write: true})
	b := WithAccess(ctx, Access{Role: "team", Issuer: "https://idp", Groups: []string{"owners-b"}, Actor: "b", Write: true})
	editor := WithAccess(ctx, Access{Role: "editor", Write: true})
	save := func(c context.Context, fn func(*Tx) error) {
		t.Helper()
		if err := st.Do(c, fn); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"a", "b"} {
		save(admin, func(tx *Tx) error {
			return tx.PutTeam(admin, Team{Slug: name, Name: name, Issuer: "https://idp", Group: "owners-" + name})
		})
		save(admin, func(tx *Tx) error { return tx.PutProject(admin, Project{Slug: name, Name: name, OwnerTeam: name}) })
	}
	owner := "a"
	cfg := restapi.Config{Auth: restapi.Auth{Type: "none"}, Operations: []restapi.Operation{{Name: "get", Enabled: true, Method: "GET", Path: "/items"}}}
	up := UpstreamInput{OwnerTeam: &owner, Kind: "rest", Name: "CRM", URL: "https://example.com", Transport: "streamable_http", Enabled: true, REST: &cfg}
	save(a, func(tx *Tx) error { _, _, err := tx.PutUpstream(a, "crm", up, nil, true); return err })
	up.OwnerTeam = nil
	save(editor, func(tx *Tx) error { _, _, err := tx.PutUpstream(editor, "public", up, nil, true); return err })
	p := "b"
	in := VirtualServerInput{Project: &p, Name: "Chat", Enabled: true, AuthRequired: true, Tools: []ToolSelection{{Upstream: "crm", Tool: "get"}, {Upstream: "public", Tool: "get"}}}
	save(b, func(tx *Tx) error { _, _, err := tx.PutVirtualServer(b, "chat", in, nil, true); return err })
	in.Project = nil
	save(editor, func(tx *Tx) error { _, _, err := tx.PutVirtualServer(editor, "legacy", in, nil, true); return err })
	if _, err := st.Reader().GetUpstream(b, "crm"); err == nil {
		t.Fatal("cross-team connection readable")
	}
	if _, err := st.Reader().GetVirtualServer(a, "chat"); err == nil {
		t.Fatal("cross-team virtual server readable")
	}
	list, _, err := st.Reader().ListVirtualServers(b, "", 1, nil)
	if err != nil || len(list) != 1 || list[0].Slug != "chat" {
		t.Fatalf("scoped pagination %v %v", list, err)
	}
	if err := st.Do(b, func(tx *Tx) error {
		_, _, e := tx.PutVirtualServer(b, "escape", VirtualServerInput{Name: "escape", Enabled: true}, nil, true)
		return e
	}); err == nil {
		t.Fatal("team created default resource")
	}
	if err := st.Do(editor, func(tx *Tx) error { _, _, e := tx.PutVirtualServer(editor, "chat", in, nil, false); return e }); err == nil {
		t.Fatal("legacy editor changed owned resource")
	}
	check := func(slug string, want bool) {
		t.Helper()
		snap, err := LoadSnapshot(ctx, pool, box)
		if err != nil {
			t.Fatal(err)
		}
		_, ok := snap.VirtualServers[slug].Lookup("crm__get")
		if ok != want {
			t.Fatalf("%s tool allowed=%v want %v", slug, ok, want)
		}
		if _, ok := snap.VirtualServers[slug].Lookup("public__get"); !ok {
			t.Fatal("unmanaged tool blocked")
		}
	}
	check("chat", false)
	check("legacy", false)
	grant := ToolGrant{VirtualServer: "chat", Upstream: "crm", Tool: "get", Status: "pending", Reason: "customer support"}
	save(b, func(tx *Tx) error { return tx.PutToolGrant(b, grant) })
	grant.Status = "approved"
	grant.Version = 1
	if err := st.Do(b, func(tx *Tx) error { return tx.PutToolGrant(b, grant) }); err == nil {
		t.Fatal("consumer approved upstream grant")
	}
	save(a, func(tx *Tx) error { return tx.PutToolGrant(a, grant) })
	check("chat", true)
	check("legacy", false)
	grant.Status = "revoked"
	grant.Version = 2
	save(a, func(tx *Tx) error { return tx.PutToolGrant(a, grant) })
	check("chat", false)
	if err := st.Do(a, func(tx *Tx) error { return tx.PutToolGrant(a, grant) }); err == nil {
		t.Fatal("stale decision accepted")
	}
	// A global viewer's team membership must not grant legacy default writes.
	viewer := WithAccess(ctx, Access{Role: "viewer", Issuer: "https://idp", Groups: []string{"owners-b"}, Write: true})
	if err := st.Do(viewer, func(tx *Tx) error { _, _, e := tx.CreateAgentKey(viewer, "empty-escape", AgentKeyInput{}); return e }); err == nil {
		t.Fatal("team viewer created unmanaged credential")
	}
	// A key cannot bridge another team's endpoint.
	if err := st.Do(b, func(tx *Tx) error {
		_, _, e := tx.CreateAgentKey(b, "escape", AgentKeyInput{VirtualServers: []string{"chat", "legacy"}})
		return e
	}); err == nil {
		t.Fatal("cross-team key created")
	}
	save(b, func(tx *Tx) error {
		_, _, e := tx.CreateAgentKey(b, "chat-key", AgentKeyInput{VirtualServers: []string{"chat"}})
		return e
	})
	if _, err := st.Reader().GetAgentKey(a, "chat-key"); err == nil {
		t.Fatal("other team read key")
	}
	// Project reassignment revokes grants and changes management visibility.
	save(admin, func(tx *Tx) error {
		return tx.PutProject(admin, Project{Slug: "b", Name: "B", OwnerTeam: "a", Version: 1})
	})
	if _, err := st.Reader().GetVirtualServer(b, "chat"); err == nil {
		t.Fatal("old owner retained access")
	}
	if _, err := st.Reader().GetVirtualServer(a, "chat"); err != nil {
		t.Fatal(err)
	}
	// Same-team automatic access never overrides an explicit revocation.
	check("chat", false)
	// New same-team tool selections work without an external approval.
	p = "a"
	in.Project = &p
	save(a, func(tx *Tx) error { _, _, e := tx.PutVirtualServer(a, "same", in, nil, true); return e })
	check("same", true)
	// Claim matching is exact and issuer scoped.
	for _, c := range []Access{{Role: "team", Issuer: "https://evil", Groups: []string{"owners-a"}}, {Role: "team", Issuer: "https://idp", Groups: []string{"owners-a' OR TRUE --"}}} {
		if _, err := st.Reader().GetUpstream(WithAccess(ctx, c), "crm"); err == nil {
			t.Fatal("invalid identity matched")
		}
	}
}
