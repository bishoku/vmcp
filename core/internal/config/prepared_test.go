package config

import (
	"encoding/json"
	"fmt"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/pii"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func preparedFixture(n int) VirtualServer {
	v := VirtualServer{}
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("tool-%d", i)
		v.Tools = append(v.Tools, VirtualTool{Tool: mcp.Tool{Name: name, Description: "Contact agent@example.com", InputSchema: json.RawMessage(`{"type":"object"}`)}})
		v.Catalog = append(v.Catalog, VirtualCatalogObject{Kind: "resource", Original: name, Exposed: name, Definition: mcp.CatalogObject{"uri": json.RawMessage(fmt.Sprintf("%q", name)), "description": json.RawMessage(`"Contact agent@example.com"`)}})
	}
	v.PII, _ = pii.Compile(pii.Config{Enabled: true, Detectors: []string{"email"}})
	return v
}
func TestPreparedMetadataMatchesFallback(t *testing.T) {
	original := preparedFixture(205)
	prepared := original.WithPreparedMetadata()
	for _, kind := range []string{"resource", "template", "prompt"} {
		old, oe := original.ListedCatalog(kind)
		got, ge := prepared.ListedCatalog(kind)
		if !reflect.DeepEqual(old, got) || !reflect.DeepEqual(oe, ge) {
			t.Fatalf("catalog mismatch: %s", kind)
		}
	}
	for _, span := range [][2]int{{0, 200}, {200, 205}, {205, 205}} {
		old, oe := original.ListedTools(span[0], span[1])
		got, ge := prepared.ListedTools(span[0], span[1])
		if !reflect.DeepEqual(old, got) || !reflect.DeepEqual(oe, ge) {
			t.Fatal("tool page mismatch")
		}
		raw, _ := json.Marshal(got)
		if strings.Contains(string(raw), "agent@example.com") {
			t.Fatal("unmasked metadata")
		}
	}
	if !reflect.DeepEqual(original.CatalogCapabilities(), prepared.CatalogCapabilities()) {
		t.Fatal("capabilities changed")
	}
	// Rebuild after a policy change; old immutable snapshots remain independently masked.
	prepared.PII = nil
	rebuilt := prepared.WithPreparedMetadata()
	listed, _ := rebuilt.ListedTools(0, 1)
	if listed[0].Description != original.Tools[0].Tool.Description {
		t.Fatal("stale policy cache")
	}
	masked, _ := original.WithPreparedMetadata().ListedTools(0, 1)
	if masked[0].Description == listed[0].Description {
		t.Fatal("snapshots share policy data")
	}
}
func TestPreparedErrorScopeAndRoutingPrecedence(t *testing.T) {
	v := preparedFixture(202)
	v.Tools[201].Tool.InputSchema = json.RawMessage(`bad`)
	v.Catalog = append(v.Catalog, VirtualCatalogObject{Kind: "resource", Exposed: "broken", Definition: mcp.CatalogObject{"uri": json.RawMessage(`bad`)}})
	v.Catalog = append(v.Catalog, VirtualCatalogObject{Kind: "template", Original: "data/{id}", Exposed: "vmcp://up/template/data/{id}"}, VirtualCatalogObject{Kind: "template", Original: "data/{other}", Exposed: "vmcp://up/template/data/{other}"}, VirtualCatalogObject{Kind: "resource", Exposed: "tool-0", Original: "duplicate"})
	p := v.WithPreparedMetadata()
	if _, err := p.ListedTools(0, 200); err != nil {
		t.Fatal("error escaped its tool page")
	}
	if _, err := p.ListedTools(200, 202); err == nil {
		t.Fatal("invalid tool accepted")
	}
	if _, err := p.ListedCatalog("resource"); err == nil {
		t.Fatal("whole-kind catalog error lost")
	}
	for _, query := range []struct {
		kind, name string
		expand     bool
	}{{"resource", "tool-0", false}, {"resource", "vmcp://up/template/data/42", true}, {"resource", "vmcp://up/template/data/42", false}, {"prompt", "absent", true}, {"resource", strings.Repeat("x", 8193), true}} {
		old, on, ok := v.CatalogRoute(query.kind, query.name, query.expand)
		got, gn, gok := p.CatalogRoute(query.kind, query.name, query.expand)
		if !reflect.DeepEqual(old, got) || on != gn || ok != gok {
			t.Fatalf("route mismatch %+v", query)
		}
	}
}
func BenchmarkPreparedCatalog(b *testing.B) {
	for _, size := range []int{100, 10000} {
		v := preparedFixture(size)
		for _, prepared := range []bool{false, true} {
			b.Run(fmt.Sprintf("objects=%d/prepared=%t", size, prepared), func(b *testing.B) {
				candidate := v
				if prepared {
					candidate = candidate.WithPreparedMetadata()
				}
				target := fmt.Sprintf("tool-%d", size-1)
				b.Run("lookup", func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						if _, _, ok := candidate.CatalogRoute("resource", target, false); !ok {
							b.Fatal("missing")
						}
					}
				})
				b.Run("metadata", func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						if _, err := candidate.ListedCatalog("resource"); err != nil {
							b.Fatal(err)
						}
					}
				})
				b.Run("tool-page", func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						if _, err := candidate.ListedTools(0, min(200, size)); err != nil {
							b.Fatal(err)
						}
					}
				})
			})
		}
	}
}

func BenchmarkPrepareSnapshotMetadata(b *testing.B) {
	v := preparedFixture(10000)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	prepared := v.WithPreparedMetadata()
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(prepared)
	runtime.KeepAlive(v)
	retained := float64(after.HeapAlloc) - float64(before.HeapAlloc)
	b.ReportAllocs()
	for b.Loop() {
		prepared := v.WithPreparedMetadata()
		if prepared.prepared == nil {
			b.Fatal("missing preparation")
		}
	}
	b.ReportMetric(retained, "retained-bytes/snapshot")
}
