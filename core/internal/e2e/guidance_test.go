package e2e

import (
	"context"
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/fixture"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/pii"
	"github.com/bishoku/vmcp/core/internal/snapshot"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/upstream"
	"github.com/bishoku/vmcp/core/internal/virtual"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLocalGuidanceProtocolPolicyAndAudit(t *testing.T) {
	e := setup(t)
	db := &store.Store{Pool: e.pool, Box: e.syncer.Box}
	g := config.Guidance{Instructions: "Use {{tool:echo}}; contact person@example.com", ToolReferences: []config.GuidanceToolReference{{Key: "echo", Upstream: "alpha", Tool: "echo"}}, Resources: []config.LocalResource{{Name: "guide", Enabled: true, MimeType: "text/markdown", Text: "Use {{tool:echo}}; person@example.com"}, {Name: "hidden", Enabled: false, MimeType: "text/plain", Text: "hidden"}}, Prompts: []config.LocalPrompt{{Name: "workflow", Enabled: true, Arguments: []config.PromptArgument{{Name: "topic", Required: true}, {Name: "extra"}}, Messages: []config.PromptMessage{{Role: "user", Text: "{{tool:echo}} {{arg:topic}} {{arg:extra}}"}}}}, UpstreamInstructions: []string{"alpha"}}
	in := store.VirtualServerInput{Name: "Guided", Enabled: true, Tools: []store.ToolSelection{{Upstream: "alpha", Tool: "echo", Alias: "say"}}, Guidance: &g, PII: pii.Config{Enabled: true, Detectors: []string{"email"}}}
	if err := db.Do(e.ctx, func(tx *store.Tx) error {
		if err := tx.ReplaceCatalog(e.ctx, "alpha", mcp.Catalog{Instructions: "Remote instructions"}); err != nil {
			return err
		}
		_, _, err := tx.PutVirtualServer(e.ctx, "guided", in, nil, false)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	wait := func(check func() bool) {
		t.Helper()
		for end := time.Now().Add(5 * time.Second); !check(); time.Sleep(10 * time.Millisecond) {
			if time.Now().After(end) {
				t.Fatal("snapshot/audit did not converge")
			}
		}
	}
	wait(func() bool { return len(e.syncer.Snapshot().VirtualServers["guided"].Catalog) == 3 })
	client := upstream.NewClient(strings.TrimSuffix(e.gwURL, "/agent")+"/guided", nil, nil)
	init, err := client.Initialize(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close(context.Background())
	if !strings.Contains(init.Instructions, "Use say") || strings.Contains(init.Instructions, "person@example.com") || init.Capabilities.Resources == nil || init.Capabilities.Prompts == nil {
		t.Fatalf("initialize: %+v", init)
	}
	call := func(method string, p any) string {
		t.Helper()
		raw, err := client.Call(e.ctx, method, p)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	listed := call("resources/list", map[string]any{})
	if !strings.Contains(listed, "vmcp://guidance/resource/guide") || strings.Contains(listed, "hidden") || !strings.Contains(listed, "vmcp://guidance/upstream/alpha") {
		t.Fatal(listed)
	}
	read := call("resources/read", map[string]any{"uri": "vmcp://guidance/resource/guide"})
	if !strings.Contains(read, "Use say") || strings.Contains(read, "person@example.com") {
		t.Fatal(read)
	}
	if raw := call("resources/read", map[string]any{"uri": "vmcp://guidance/upstream/alpha"}); !strings.Contains(raw, "Remote instructions") {
		t.Fatal(raw)
	}
	rendered := call("prompts/get", map[string]any{"name": "vmcp_local__workflow", "arguments": map[string]string{"topic": "{{tool:secret}} person@example.com"}})
	if !strings.Contains(rendered, "say {{tool:secret}}") || strings.Contains(rendered, "person@example.com") {
		t.Fatal(rendered)
	}
	for _, p := range []map[string]any{{"name": "vmcp_local__workflow"}, {"name": "vmcp_local__workflow", "arguments": map[string]string{"topic": "x", "unknown": "y"}}} {
		if _, err := client.Call(e.ctx, "prompts/get", p); err == nil {
			t.Fatal("invalid arguments accepted")
		}
	}
	if _, err := client.Call(e.ctx, "resources/read", map[string]any{"uri": "vmcp://guidance/resource/hidden"}); err == nil {
		t.Fatal("disabled resource exposed")
	}
	wait(func() bool {
		var n int
		_ = e.pool.QueryRow(e.ctx, `SELECT count(*) FROM audit_events WHERE virtual_server='guided' AND method IN ('resources/read','prompts/get') AND status='ok'`).Scan(&n)
		return n >= 3
	})
	// Alias updates recompile existing guidance; policy-hidden tools suppress dependent items.
	e.exec(t, `UPDATE virtual_server_tools SET alias='renamed' WHERE virtual_server_id=(SELECT id FROM virtual_servers WHERE slug='guided')`)
	wait(func() bool {
		return strings.Contains(e.syncer.Snapshot().VirtualServers["guided"].Instructions, "Use renamed")
	})
	if raw := call("resources/read", map[string]any{"uri": "vmcp://guidance/resource/guide"}); !strings.Contains(raw, "renamed") {
		t.Fatal(raw)
	}
	e.exec(t, `UPDATE virtual_servers SET tool_policy='{"default":"deny","tools":{}}'::jsonb WHERE slug='guided'`)
	wait(func() bool { return len(e.syncer.Snapshot().VirtualServers["guided"].Catalog) == 1 })
	if _, err := client.Call(e.ctx, "resources/read", map[string]any{"uri": "vmcp://guidance/resource/guide"}); err == nil {
		t.Fatal("policy blocked tool leaked guidance")
	}
	// A server with no upstream selections still advertises local capabilities.
	var resource map[string]any
	_ = json.Unmarshal([]byte(call("resources/read", map[string]any{"uri": "vmcp://guidance/upstream/alpha"})), &resource)
	if resource["contents"] == nil {
		t.Fatal(resource)
	}
}

func TestMixedGuidanceAcrossNodesAndAccessRevocation(t *testing.T) {
	e := setup(t)
	physical := httptest.NewServer(fixture.NewCatalog())
	defer physical.Close()
	remote := upstream.NewClient(physical.URL, nil, nil)
	init, err := remote.Initialize(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := upstream.DiscoverCatalog(e.ctx, remote, init.Capabilities)
	if err != nil {
		t.Fatal(err)
	}
	_ = remote.Close(e.ctx)
	catalog.Instructions = "Remote workflow"
	db := &store.Store{Pool: e.pool, Box: e.syncer.Box}
	in := store.VirtualServerInput{Name: "Mixed", Enabled: true, Catalog: []store.CatalogSelection{{Upstream: "catalog", Kind: "resource", Name: "doc://public"}, {Upstream: "catalog", Kind: "template", Name: "doc://items/{id}"}, {Upstream: "catalog", Kind: "prompt", Name: "review"}}, Guidance: &config.Guidance{Resources: []config.LocalResource{{Name: "local", Enabled: true, MimeType: "text/plain", Text: "Local workflow"}}, Prompts: []config.LocalPrompt{{Name: "local", Enabled: true, Messages: []config.PromptMessage{{Role: "user", Text: "Local prompt"}}}}, UpstreamInstructions: []string{"catalog"}}}
	if err := db.Do(e.ctx, func(tx *store.Tx) error {
		if _, _, err := tx.PutUpstream(e.ctx, "catalog", store.UpstreamInput{Name: "Catalog", URL: physical.URL, Transport: "streamable_http", Enabled: true}, nil, false); err != nil {
			return err
		}
		if err := tx.ReplaceCatalog(e.ctx, "catalog", catalog); err != nil {
			return err
		}
		_, _, err := tx.PutVirtualServer(e.ctx, "mixed", in, nil, false)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(e.ctx)
	defer cancel()
	second := &snapshot.Syncer{Pool: e.pool, Box: e.syncer.Box, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), PollInterval: time.Hour}
	if err := second.Start(ctx); err != nil {
		t.Fatal(err)
	}
	manager := virtual.NewManager()
	defer manager.Close()
	mux := http.NewServeMux()
	(&virtual.Handler{Snap: second, Sessions: manager, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Register(mux)
	node := httptest.NewServer(mux)
	defer node.Close()
	wait := func(check func() bool) {
		t.Helper()
		for end := time.Now().Add(5 * time.Second); !check(); time.Sleep(10 * time.Millisecond) {
			if time.Now().After(end) {
				t.Fatal("snapshots did not converge")
			}
		}
	}
	wait(func() bool {
		return len(e.syncer.Snapshot().VirtualServers["mixed"].Catalog) == 6 && len(second.Snapshot().VirtualServers["mixed"].Catalog) == 6
	})
	var clients []*upstream.Client
	for _, url := range []string{strings.TrimSuffix(e.gwURL, "/agent") + "/mixed", node.URL + "/mcp/v1/virtual/mixed"} {
		c := upstream.NewClient(url, nil, nil)
		clients = append(clients, c)
		defer c.Close(context.Background())
		if _, err := c.Initialize(e.ctx); err != nil {
			t.Fatal(err)
		}
		for _, method := range []string{"resources/list", "resources/templates/list", "prompts/list"} {
			raw, err := c.Call(e.ctx, method, map[string]any{})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), "catalog") {
				t.Fatalf("upstream capability lost: %s", raw)
			}
		}
		for _, uri := range []string{config.LocalResourceURI("local"), config.CatalogName("catalog", "resource", "doc://public", ""), config.UpstreamInstructionsURI("catalog")} {
			if _, err := c.Call(e.ctx, "resources/read", map[string]any{"uri": uri}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := c.Call(e.ctx, "prompts/get", map[string]any{"name": "catalog__review", "arguments": map[string]string{"subject": "x"}}); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Call(e.ctx, "prompts/get", map[string]any{"name": "vmcp_local__local"}); err != nil {
			t.Fatal(err)
		}
	}
	// Requiring authentication revokes existing anonymous access to local content on both nodes.
	e.exec(t, `UPDATE virtual_servers SET auth_required=true WHERE slug='mixed'`)
	wait(func() bool {
		return e.syncer.Snapshot().VirtualServers["mixed"].AuthRequired && second.Snapshot().VirtualServers["mixed"].AuthRequired
	})
	for _, c := range clients {
		if _, err := c.Call(e.ctx, "resources/read", map[string]any{"uri": config.LocalResourceURI("local")}); err == nil {
			t.Fatal("anonymous local access survived revocation")
		}
	}
}
