package e2e

import (
	"context"
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/upstream"
	"testing"
	"time"
)

func TestToolMetadataPersistsThroughDiscoveryAndComposition(t *testing.T) {
	e := setup(t)
	database := &store.Store{Pool: e.pool, Box: e.syncer.Box}
	rich := mcp.Tool{Name: "echo", Title: "Echo display", Description: "upstream description", InputSchema: json.RawMessage(`{"type":"object"}`), OutputSchema: json.RawMessage(`{"type":"object","properties":{"answer":{"type":"string"}}}`), Meta: json.RawMessage(`{"revision":"metadata-1"}`), Extra: map[string]json.RawMessage{"x-vendor": json.RawMessage(`{"supported":true}`)}}
	if err := database.Do(e.ctx, func(tx *store.Tx) error {
		_, err := tx.ReplaceTools(e.ctx, "alpha", []mcp.Tool{rich}, time.Now())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for end := time.Now().Add(3 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if e.syncer.Snapshot().VirtualServers["agent"].Tools[0].Tool.Title == "Echo display" {
			break
		}
		if time.Now().After(end) {
			t.Fatal("snapshot dropped tool metadata")
		}
	}
	stored, err := database.Reader().ListTools(e.ctx, "alpha")
	if err != nil || len(stored) != 1 || stored[0].Title != "Echo display" || len(stored[0].OutputSchema) == 0 || len(stored[0].Meta) == 0 {
		t.Fatalf("cached metadata lost: %+v %v", stored, err)
	}
	client := upstream.NewClient(e.gwURL, nil, nil)
	if _, err := client.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer client.Close(context.Background())
	tools, err := client.ListTools(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range tools {
		if tool.Name == "say" {
			found = tool.Title == "Echo display" && len(tool.OutputSchema) > 0 && len(tool.Meta) > 0 && len(tool.Extra["x-vendor"]) > 0
		}
	}
	if !found {
		t.Fatal("curated tools/list dropped metadata or alias")
	}
	revision, err := store.Revision(e.ctx, e.pool)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Do(e.ctx, func(tx *store.Tx) error {
		result, err := tx.ReplaceTools(e.ctx, "alpha", []mcp.Tool{rich}, time.Now())
		if err == nil && len(result.Changed) > 0 {
			t.Fatal("unchanged metadata causes repeated writes")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	after, _ := store.Revision(e.ctx, e.pool)
	if after != revision {
		t.Fatal("idle discovery bumped revision")
	}
}
