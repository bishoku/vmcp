//go:build acceptance

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/jackc/pgx/v5/pgxpool"
)

func acceptanceRSSGrowth(values []float64) float64 {
	if len(values) < 6 {
		return 0
	}
	n := len(values) / 3
	first, last := append([]float64(nil), values[:n]...), append([]float64(nil), values[len(values)-n:]...)
	sort.Float64s(first)
	sort.Float64s(last)
	a, b := acceptanceQuantile(first, .5), acceptanceQuantile(last, .5)
	if a <= 0 {
		return 0
	}
	return (b - a) * 100 / a
}

func acceptanceLoadedRevocationClient(t *testing.T, ctx context.Context, node *acceptanceNode, token string, client *http.Client) *acceptanceClient {
	t.Helper()
	probe := &acceptanceClient{url: node.url + "/mcp/v1/virtual/accept-vs-0000", token: token, http: client}
	if r := probe.request(ctx, mcp.MethodInitialize, map[string]any{"protocolVersion": mcp.SupportedVersions[0], "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "revocation-probe", "version": "1"}}, -100); r.err != nil {
		t.Fatal(r.err)
	}
	if r := probe.request(ctx, mcp.MethodInitialized, nil, 0); r.err != nil {
		t.Fatal(r.err)
	}
	if r := probe.request(ctx, mcp.MethodToolsList, map[string]any{}, -101); r.err != nil {
		t.Fatal(r.err)
	}
	return probe
}

func acceptanceLoadedRevocation(ctx context.Context, pool *pgxpool.Pool, node *acceptanceNode, probe *acceptanceClient) (map[string]any, error) {
	evidence := map[string]any{"key_slug": "accept-key-64", "budget_seconds": 5}
	updateCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := pool.Exec(updateCtx, `UPDATE agent_keys SET revoked_at=now() WHERE slug='accept-key-64'`); err != nil {
		return evidence, err
	}
	started := time.Now()
	evidence["committed_utc"] = started.UTC().Format(time.RFC3339Nano)
	var observations []map[string]any
	// Continue probing after first rejection to detect acceptance regressions.
	deadline := started.Add(10 * time.Second)
	firstRejection := time.Duration(0)
	healthy, stable := true, true
	client := &http.Client{Timeout: time.Second}
	for time.Now().Before(deadline) {
		bounded, done := context.WithTimeout(ctx, time.Second)
		response := probe.request(bounded, mcp.MethodToolsList, map[string]any{}, -102)
		done()
		elapsed := time.Since(started)
		readyStatus := 0
		ready, err := client.Get(node.url + "/readyz")
		if err == nil {
			readyStatus = ready.StatusCode
			ready.Body.Close()
		}
		if readyStatus != 200 {
			healthy = false
		}
		observations = append(observations, map[string]any{"seconds": elapsed.Seconds(), "status": response.status, "ready_status": readyStatus})
		if response.status == 401 && firstRejection == 0 {
			firstRejection = elapsed
		}
		if firstRejection > 0 && response.status != 401 {
			stable = false
		}
		if response.status != 200 && response.status != 401 {
			stable = false
		}
		time.Sleep(100 * time.Millisecond)
	}
	evidence["observations"] = observations
	evidence["first_401_seconds"] = firstRejection.Seconds()
	evidence["readiness_healthy"] = healthy
	evidence["rejection_stable"] = stable
	if firstRejection == 0 || firstRejection > 5*time.Second || !healthy || !stable {
		return evidence, fmt.Errorf("loaded revocation failed: first401=%s healthy=%v stable=%v", firstRejection, healthy, stable)
	}
	return evidence, nil
}

func acceptanceKubeNoRestarts(ctx context.Context) error {
	raw, err := acceptanceKubectl(ctx, nil, "get", "pods", "-o", "json")
	if err != nil {
		return err
	}
	var pods struct {
		Items []struct {
			Metadata struct{ Name string }
			Status   struct {
				ContainerStatuses []struct {
					Name         string
					RestartCount int
					LastState    struct{ Terminated struct{ Reason string } }
				}
			}
		}
	}
	if err = json.Unmarshal(raw, &pods); err != nil {
		return err
	}
	found := false
	for _, pod := range pods.Items {
		for _, container := range pod.Status.ContainerStatuses {
			if container.Name != "data-plane" {
				continue
			}
			found = true
			if container.RestartCount != 0 || container.LastState.Terminated.Reason == "OOMKilled" {
				return fmt.Errorf("data-plane restarted or OOMKilled: %s", pod.Metadata.Name)
			}
		}
	}
	if !found {
		return fmt.Errorf("data-plane restart evidence missing")
	}
	return nil
}

func TestAcceptanceMemoryGrowth(t *testing.T) {
	if got := acceptanceRSSGrowth([]float64{100, 100, 500, 500, 125, 125}); got != 25 {
		t.Fatalf("growth=%g", got)
	}
	samples := []acceptanceSample{}
	for _, rss := range []int64{100, 100, 500, 500, 125, 125} {
		samples = append(samples, acceptanceSample{Phase: "steady", Nodes: []acceptanceNodeSample{{Name: "n", Alive: true, RSSBytes: rss, MemoryUsageBytes: rss + 10}}})
	}
	r := acceptanceResources(samples)["n"].(map[string]any)
	if r["steady_rss_growth_percent"] != float64(25) || r["max_container_usage_bytes"] != float64(510) {
		t.Fatal(r)
	}
}
