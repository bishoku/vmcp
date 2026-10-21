package store

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/bishoku/vmcp/core/internal/callercontext"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/restapi"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/testdb"
)

func TestRESTSnapshotReuseInvalidation(t *testing.T) {
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
	box, err := secrets.NewBox(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	st := Store{Pool: pool, Box: box}
	cfg := restapi.Config{Auth: restapi.Auth{Type: "none"}, Operations: []restapi.Operation{{Name: "get", Enabled: true, Method: "GET", Path: "/items"}}}
	in := UpstreamInput{Kind: "rest", Name: "REST", URL: "https://example.com", Transport: "streamable_http", Enabled: true, REST: &cfg}
	save := func() {
		t.Helper()
		if err := st.Do(ctx, func(tx *Tx) error { _, _, err := tx.PutUpstream(ctx, "rest", in, nil, false); return err }); err != nil {
			t.Fatal(err)
		}
	}
	load := func(prev *config.Snapshot) *config.Snapshot {
		t.Helper()
		s, err := LoadSnapshotWithPrevious(ctx, pool, box, prev)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	save()
	first := load(nil)
	var id string
	for key := range first.Upstreams {
		id = key
	}
	same := load(first)
	if same.Upstreams[id].REST != first.Upstreams[id].REST {
		t.Fatal("unchanged schemas recompiled")
	}
	in.Name = "Renamed"
	save()
	same = load(same)
	if same.Upstreams[id].REST != first.Upstreams[id].REST {
		t.Fatal("metadata invalidated schemas")
	}
	beforeContext := same
	in.CallerContext = &callercontext.Config{Enabled: true, MetaKey: "bank/context", Fields: []callercontext.Field{{Name: "token", Header: "X-Token"}}}
	save()
	same = load(beforeContext)
	if same.Upstreams[id].CallerContext == nil || same.Upstreams[id].CallerContext.MetaKey != "bank/context" {
		t.Fatal("context missing from runtime snapshot")
	}
	if same.Revision <= beforeContext.Revision {
		t.Fatal("context update did not publish a revision")
	}
	if same.Upstreams[id].Fingerprint() == beforeContext.Upstreams[id].Fingerprint() {
		t.Fatal("context update did not invalidate upstream fingerprint")
	}
	for _, change := range []func(){func() { in.URL = "https://other.example.com" }, func() { in.Headers = map[string]string{"X-Key": "changed"} }, func() { cfg.Operations[0].Path = "/changed" }, func() { cfg.Auth = restapi.Auth{Type: "bearer", Token: "new-secret"} }} {
		before := same
		change()
		save()
		same = load(before)
		if same.Upstreams[id].REST == before.Upstreams[id].REST {
			t.Fatal("changed REST input reused stale schemas")
		}
	}
	if first.Upstreams[id].REST.Config.Operations[0].Path != "/items" {
		t.Fatal("previous snapshot mutated")
	}
	in.Enabled = false
	save()
	disabled := load(same)
	if len(disabled.Upstreams) != 0 {
		t.Fatal("disabled upstream retained")
	}
	in.Enabled = true
	save()
	if _, err := pool.Exec(ctx, `UPDATE upstreams SET encrypted_rest_config=decode('0102','hex') WHERE slug='rest'`); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSnapshotWithPrevious(ctx, pool, box, same); err == nil {
		t.Fatal("reuse bypassed authenticated decryption")
	}
}
