package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/delivery"
	"github.com/bishoku/vmcp/core/internal/fixture"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/pii"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/upstream"
	"github.com/bishoku/vmcp/core/internal/virtual"
)

func TestCuratedCatalogProtocolAndCrossNodeSubscriptions(t *testing.T) {
	e := setup(t)
	physicalHandler := fixture.NewCatalog()
	physical := httptest.NewServer(physicalHandler)
	defer physical.Close()
	database := &store.Store{Pool: e.pool, Box: e.syncer.Box}
	for _, slug := range []string{"catalog-a", "catalog-b"} {
		c := upstream.NewClient(physical.URL, nil, nil)
		init, err := c.Initialize(e.ctx)
		if err != nil {
			t.Fatal(err)
		}
		catalog, err := upstream.DiscoverCatalog(e.ctx, c, init.Capabilities)
		if err != nil {
			t.Fatal(err)
		}
		_ = c.Close(e.ctx)
		err = database.Do(e.ctx, func(tx *store.Tx) error {
			if _, _, err := tx.PutUpstream(e.ctx, slug, store.UpstreamInput{Name: slug, URL: physical.URL, Transport: "streamable_http", Enabled: true}, nil, false); err != nil {
				return err
			}
			return tx.ReplaceCatalog(e.ctx, slug, catalog)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	selections := []store.CatalogSelection{{Upstream: "catalog-a", Kind: "resource", Name: "doc://public"}, {Upstream: "catalog-b", Kind: "resource", Name: "doc://public"}, {Upstream: "catalog-a", Kind: "resource", Name: "doc://binary"}, {Upstream: "catalog-a", Kind: "template", Name: "doc://items/{id}"}, {Upstream: "catalog-a", Kind: "prompt", Name: "review", Alias: "approved_review"}}
	if err := database.Do(e.ctx, func(tx *store.Tx) error {
		_, _, err := tx.PutVirtualServer(e.ctx, "catalog-agent", store.VirtualServerInput{Name: "Catalog Agent", Enabled: true, PII: pii.Config{Enabled: true, Detectors: []string{"email"}}, Catalog: selections}, nil, false)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for end := time.Now().Add(3 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if len(e.syncer.Snapshot().VirtualServers["catalog-agent"].Catalog) == 5 {
			break
		}
		if time.Now().After(end) {
			t.Fatal("snapshot did not load curated catalogs")
		}
	}
	broker := &delivery.Store{Pool: e.pool, Box: e.syncer.Box}
	newNode := func() (*virtual.Handler, *httptest.Server) {
		manager := virtual.NewManager()
		manager.Store = virtual.PGSessionStore{Pool: e.pool}
		h := &virtual.Handler{Snap: e.syncer, Sessions: manager, Delivery: broker, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
		mux := http.NewServeMux()
		h.Register(mux)
		srv := httptest.NewServer(mux)
		t.Cleanup(func() {
			h.StopWorkers()
			bounded, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = h.WaitWorkers(bounded)
			manager.Close()
			srv.Close()
		})
		return h, srv
	}
	nodeA, serverA := newNode()
	nodeB, serverB := newNode()
	_ = serverB
	workersB, stopB := context.WithCancel(e.ctx)
	nodeB.RunSubscriptions(workersB)
	defer stopB()
	client := upstream.NewClient(serverA.URL+"/mcp/v1/virtual/catalog-agent", nil, nil)
	init, err := client.Initialize(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if init.Capabilities.Resources == nil || !init.Capabilities.Resources.Subscribe || init.Capabilities.Prompts == nil {
		t.Fatal("selected capabilities absent")
	}
	defer client.Close(context.Background())
	raw, err := client.Call(e.ctx, "resources/list", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	var resources struct {
		Resources []mcp.CatalogObject `json:"resources"`
	}
	_ = json.Unmarshal(raw, &resources)
	if len(resources.Resources) != 3 || resources.Resources[0].String("uri") == resources.Resources[1].String("uri") || resources.Resources[0].String("title") != "Public title" || len(resources.Resources[0]["_meta"]) == 0 {
		t.Fatalf("curation/metadata lost: %s", raw)
	}
	exposed := config.CatalogName("catalog-a", "resource", "doc://public", "")
	raw, err = client.Call(e.ctx, "resources/read", map[string]string{"uri": exposed})
	if err != nil || strings.Contains(string(raw), "person@example.com") || !strings.Contains(string(raw), exposed) {
		t.Fatalf("read routing/privacy: %s %v", raw, err)
	}
	reads := physicalHandler.Reads.Load()
	for _, uri := range []string{"doc://hidden", "https://127.0.0.1/secret", config.CatalogName("catalog-a", "resource", "doc://hidden", "")} {
		if _, err := client.Call(e.ctx, "resources/read", map[string]string{"uri": uri}); err == nil {
			t.Fatal("denied resource directly readable")
		}
	}
	if physicalHandler.Reads.Load() != reads {
		t.Fatal("denied resource reached physical server")
	}
	if _, err := client.Call(e.ctx, "resources/read", map[string]string{"uri": config.CatalogName("catalog-a", "resource", "doc://binary", "")}); err == nil {
		t.Fatal("uninspectable binary escaped PII policy")
	}
	raw, err = client.Call(e.ctx, "resources/read", map[string]string{"uri": "vmcp://catalog-a/template/doc://items/42"})
	if err != nil || !strings.Contains(string(raw), "items/42") {
		t.Fatalf("template read %s %v", raw, err)
	}
	if _, err := client.Call(e.ctx, "prompts/get", map[string]any{"name": "approved_review"}); err == nil {
		t.Fatal("required prompt argument ignored")
	}
	raw, err = client.Call(e.ctx, "prompts/get", map[string]any{"name": "approved_review", "arguments": map[string]string{"subject": "topic"}})
	if err != nil || strings.Contains(string(raw), "person@example.com") {
		t.Fatalf("prompt privacy %s %v", raw, err)
	}
	if _, err := client.Call(e.ctx, "prompts/get", map[string]any{"name": "review", "arguments": map[string]string{"subject": "topic"}}); err == nil {
		t.Fatal("physical prompt name bypassed curation")
	}
	raw, err = client.Call(e.ctx, "completion/complete", map[string]any{"ref": map[string]string{"type": "ref/prompt", "name": "approved_review"}, "argument": map[string]string{"name": "subject", "value": "a"}})
	if err != nil || !strings.Contains(string(raw), "alpha") {
		t.Fatalf("completion %s %v", raw, err)
	}
	if _, err := client.Call(e.ctx, "resources/subscribe", map[string]string{"uri": exposed}); err != nil {
		t.Fatal(err)
	}
	var owner string
	if err := e.pool.QueryRow(e.ctx, `SELECT owner::text FROM mcp_resource_subscriptions WHERE session_id=$1 AND uri=$2 AND ready`, client.SessionID(), exposed).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	// The physical subscription runs on B while the client GET is served by A.
	getCtx, stopGET := context.WithTimeout(e.ctx, 5*time.Second)
	defer stopGET()
	req, _ := http.NewRequestWithContext(getCtx, "GET", serverA.URL+"/mcp/v1/virtual/catalog-agent", nil)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Mcp-Session-Id", client.SessionID())
	req.Header.Set("MCP-Protocol-Version", mcp.ProtocolVersion20250618)
	stream, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	physicalHandler.Update("doc://public")
	scan := bufio.NewScanner(stream.Body)
	found := false
	for scan.Scan() {
		if strings.Contains(scan.Text(), "notifications/resources/updated") && strings.Contains(scan.Text(), exposed) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("cross-node resource notification missing")
	}
	stream.Body.Close()
	stopB()
	workersA, stopA := context.WithCancel(e.ctx)
	defer stopA()
	nodeA.RunSubscriptions(workersA)
	for end := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		var changed bool
		_ = e.pool.QueryRow(e.ctx, `SELECT ready AND owner<>$3::uuid FROM mcp_resource_subscriptions WHERE session_id=$1 AND uri=$2`, client.SessionID(), exposed, owner).Scan(&changed)
		if changed {
			break
		}
		if time.Now().After(end) {
			t.Fatal("subscription did not recover on surviving node")
		}
	}
	if _, err := client.Call(e.ctx, "resources/unsubscribe", map[string]string{"uri": exposed}); err != nil {
		t.Fatal(err)
	}
	var count int
	_ = e.pool.QueryRow(e.ctx, `SELECT count(*) FROM mcp_resource_subscriptions WHERE session_id=$1`, client.SessionID()).Scan(&count)
	if count != 0 {
		t.Fatal("unsubscribe retained delivery grant")
	}
}
