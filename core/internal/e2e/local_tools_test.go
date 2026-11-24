package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/localtools"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/upstream"
)

func TestLocalToolPublishedAndInvokedWithGuidance(t *testing.T) {
	e := setup(t)
	db := &store.Store{Pool: e.pool, Box: e.syncer.Box}
	defs := localtools.Config{Tools: []localtools.Definition{{Name: "details", Parameters: []localtools.Parameter{{Name: "names", Type: "string[]", Required: true}, {Name: "examples", Type: "boolean", Default: []byte("false")}}, Response: "{{range .Args.names}}{{.}}{{if $.Args.examples}} example{{end}};{{end}}"}}}
	guidance := config.Guidance{Instructions: "Use {{tool:details}}", ToolReferences: []config.GuidanceToolReference{{Key: "details", Upstream: "standards", Tool: "details"}}}
	if err := db.Do(e.ctx, func(tx *store.Tx) error {
		u, _, err := tx.PutUpstream(e.ctx, "standards", store.UpstreamInput{Name: "Standards", Enabled: true, Transport: "streamable_http", Kind: "local", Local: &defs}, nil, true)
		if err != nil {
			return err
		}
		if u.ToolCount != 1 || u.Local == nil {
			t.Errorf("catalog missing after save: %+v", u)
		}
		_, _, err = tx.PutVirtualServer(e.ctx, "local-ui", store.VirtualServerInput{Name: "Local UI", Enabled: true, Tools: []store.ToolSelection{{Upstream: "standards", Tool: "details", Alias: "component_details"}}, Guidance: &guidance}, nil, true)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for end := time.Now().Add(5 * time.Second); len(e.syncer.Snapshot().VirtualServers["local-ui"].Tools) != 1; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatal("local catalog did not converge")
		}
	}
	client := upstream.NewClient(strings.TrimSuffix(e.gwURL, "/agent")+"/local-ui", nil, nil)
	init, err := client.Initialize(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close(context.Background())
	if init.Instructions != "Use component_details" {
		t.Fatal(init.Instructions)
	}
	for args, want := range map[string]string{`{"names":["Button"]}`: "Button;", `{"names":["Button","TextInput"],"examples":true}`: "Button example;TextInput example;"} {
		res := call(t, client, "component_details", args)
		if res.IsError || len(res.Content) != 1 || res.Content[0].Text != want {
			t.Fatal(res)
		}
	}
}
