package store

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/bishoku/vmcp/core/internal/restapi"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/testdb"
)

func TestBulkToolGrantsIsolatedDecisionsAndVersions(t *testing.T) {
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
	owner := WithAccess(ctx, Access{Role: "team", Issuer: "https://idp", Groups: []string{"owner"}, Actor: "owner", Write: true})
	consumer := WithAccess(ctx, Access{Role: "team", Issuer: "https://idp", Groups: []string{"consumer"}, Actor: "consumer", Write: true})
	save := func(fn func(*Tx) error) {
		t.Helper()
		if err := st.Do(admin, fn); err != nil {
			t.Fatal(err)
		}
	}
	for _, slug := range []string{"owner", "consumer"} {
		save(func(tx *Tx) error {
			return tx.PutTeam(admin, Team{Slug: slug, Name: slug, Issuer: "https://idp", Group: slug})
		})
		save(func(tx *Tx) error { return tx.PutProject(admin, Project{Slug: slug, Name: slug, OwnerTeam: slug}) })
	}
	team, project := "owner", "consumer"
	cfg := restapi.Config{Auth: restapi.Auth{Type: "none"}, Operations: []restapi.Operation{{Name: "get", Enabled: true, Method: "GET", Path: "/get"}, {Name: "list", Enabled: true, Method: "GET", Path: "/list"}}}
	save(func(tx *Tx) error {
		_, _, err := tx.PutUpstream(admin, "crm", UpstreamInput{OwnerTeam: &team, Kind: "rest", Name: "CRM", URL: "https://example.com", Transport: "streamable_http", Enabled: true, REST: &cfg}, nil, true)
		return err
	})
	save(func(tx *Tx) error {
		_, _, err := tx.PutVirtualServer(admin, "chat", VirtualServerInput{Project: &project, Name: "Chat", Enabled: true, Tools: []ToolSelection{{Upstream: "crm", Tool: "get"}, {Upstream: "crm", Tool: "list"}}}, nil, true)
		return err
	})
	grants := []ToolGrant{{VirtualServer: "chat", Upstream: "crm", Tool: "get", Status: "pending", Reason: "Needed"}, {VirtualServer: "chat", Upstream: "crm", Tool: "list", Status: "pending", Reason: "Needed"}}
	batch := func(c context.Context, items []ToolGrant, changed, unchanged, failed int) BulkToolGrantResult {
		t.Helper()
		r, err := st.BulkToolGrants(c, items)
		if err != nil || r.Changed != changed || r.Unchanged != unchanged || r.Failed != failed {
			t.Fatalf("batch: %+v %v", r, err)
		}
		return r
	}
	missing := ToolGrant{VirtualServer: "chat", Upstream: "crm", Tool: "missing", Status: "pending"}
	r := batch(consumer, append(append([]ToolGrant{}, grants...), missing), 2, 0, 1)
	if r.Items[2].Code != "not_found" {
		t.Fatalf("missing: %+v", r)
	}
	for i := range grants {
		grants[i].Status = "approved"
		grants[i].Version = 1
	}
	r = batch(consumer, grants, 0, 0, 2)
	if r.Items[0].Code != "forbidden" {
		t.Fatal("consumer approved upstream grant")
	}
	grants[1].Version = 99
	r = batch(owner, grants, 1, 0, 1)
	if r.Items[1].Code != "precondition_failed" {
		t.Fatal("stale version accepted")
	}
	grants[0].Version = 2
	batch(owner, grants[:1], 0, 1, 0)
	viewer := WithAccess(ctx, Access{Role: "viewer", Issuer: "https://idp", Groups: []string{"owner"}, Write: false})
	if _, err := st.BulkToolGrants(viewer, grants[:1]); err == nil {
		t.Fatal("viewer permitted to write")
	}
	grants[1].Version = 1
	batch(owner, grants[1:], 1, 0, 0)
	snap, err := LoadSnapshot(ctx, pool, box)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.VirtualServers["chat"].Tools) != 2 {
		t.Fatal("approved tools missing from runtime snapshot")
	}
	grants[0].Status = "revoked"
	grants[0].Reason = "No longer required"
	batch(owner, grants[:1], 1, 0, 0)
	// A stale retry cannot replay a decision, and consumer requests cannot overwrite approvals.
	batch(owner, grants[:1], 0, 0, 1)
	grants[1].Status = "pending"
	grants[1].Version = 2
	batch(consumer, grants[1:], 0, 0, 1)
	snap, err = LoadSnapshot(ctx, pool, box)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.VirtualServers["chat"].Tools) != 1 {
		t.Fatal("revoked tool remained published")
	}
	for _, items := range [][]ToolGrant{nil, make([]ToolGrant, 201), {grants[0], grants[0]}, {{Status: "rejected"}}} {
		if _, err := st.BulkToolGrants(admin, items); err == nil {
			t.Fatal("invalid batch accepted")
		}
	}
}
