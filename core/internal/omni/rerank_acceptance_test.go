//go:build acceptance

package omni

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

type measuredCandidate struct {
	Name  string  `json:"name"`
	Ref   string  `json:"tool_ref"`
	Base  float64 `json:"retrieval_score"`
	Score float64 `json:"rerank_score"`
}
type rerankMeasurement struct {
	Case       qualityCase         `json:"case"`
	Split      string              `json:"split"`
	Candidates []measuredCandidate `json:"candidates"`
	ElapsedMS  float64             `json:"rerank_elapsed_ms"`
}
type relevanceMetrics struct {
	Queries        int     `json:"queries"`
	Hit1           int     `json:"hit1"`
	Hit5           int     `json:"hit5"`
	MRR            float64 `json:"mrr_at_5"`
	FalsePositives int     `json:"false_positive_queries"`
	Empty          int     `json:"empty_queries"`
}

func evaluateMeasurements(rows []rerankMeasurement, split string, window int, threshold float64, rerank bool) map[string]relevanceMetrics {
	totals := map[string]relevanceMetrics{}
	for _, row := range rows {
		if split != "all" && row.Split != split {
			continue
		}
		group := row.Case.Language
		if len(row.Case.Expected) == 0 {
			group = "negative"
		}
		m := totals[group]
		m.Queries++
		candidates := append([]measuredCandidate(nil), row.Candidates[:min(window, len(row.Candidates))]...)
		if rerank {
			sort.Slice(candidates, func(a, b int) bool {
				if candidates[a].Score == candidates[b].Score {
					return candidates[a].Ref < candidates[b].Ref
				}
				return candidates[a].Score > candidates[b].Score
			})
		}
		selected := []measuredCandidate{}
		for _, c := range candidates {
			if rerank && c.Score < threshold {
				continue
			}
			selected = append(selected, c)
			if len(selected) == 5 {
				break
			}
		}
		rank := 0
		for n, c := range selected {
			for _, expected := range row.Case.Expected {
				if c.Name == expected && (rank == 0 || n+1 < rank) {
					rank = n + 1
				}
			}
		}
		if rank == 1 {
			m.Hit1++
		}
		if rank > 0 {
			m.Hit5++
			m.MRR += 1 / float64(rank)
		}
		if len(selected) == 0 {
			m.Empty++
		}
		if group == "negative" && len(selected) > 0 {
			m.FalsePositives++
		}
		totals[group] = m
	}
	for group, m := range totals {
		if m.Queries > 0 {
			m.MRR /= float64(m.Queries)
		}
		totals[group] = m
	}
	return totals
}
func noRegression(candidate, baseline map[string]relevanceMetrics) bool {
	for _, lang := range []string{"en", "tr"} {
		a, b := candidate[lang], baseline[lang]
		if a.Hit1 < b.Hit1 || a.Hit5 < b.Hit5 || a.MRR+1e-9 < b.MRR {
			return false
		}
	}
	return candidate["negative"].FalsePositives <= baseline["negative"].FalsePositives
}
func TestRerankQualityCalibration(t *testing.T) {
	endpoint, rerankEndpoint := os.Getenv("VMCP_TEST_EMBEDDING_URL"), os.Getenv("VMCP_TEST_RERANK_URL")
	if endpoint == "" || rerankEndpoint == "" {
		t.Skip("real embedding and rerank endpoints required")
	}
	corpus := loadQualityCorpus(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Catalog{Documents: corpus.Documents})
	}))
	defer server.Close()
	backend, _ := NewBackend(server.URL, strings.Repeat("x", 32))
	embedder, err := NewHTTPEmbedder(endpoint, os.Getenv("VMCP_TEST_EMBEDDING_MODEL"), "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer embedder.Client.CloseIdleConnections()
	index := NewIndex(embedder)
	index.Refresh(context.Background(), backend)
	if index.Status().Error != "" {
		t.Fatal(index.Status())
	}
	access := Access{Refs: map[string]string{}}
	for _, d := range corpus.Documents {
		access.Refs[d.Ref] = d.Hash()
	}
	reranker, err := NewHTTPReranker(rerankEndpoint, os.Getenv("VMCP_TEST_RERANK_MODEL"), "", false, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer reranker.Client.CloseIdleConnections()
	// Warm-up is separate from measured requests. Product default timeout stays 5s.
	if _, err := reranker.Rerank(context.Background(), "Find orders", []Document{corpus.Documents[0]}); err != nil {
		t.Fatal(err)
	}
	rows := []rerankMeasurement{}
	durations := []float64{}
	for n, q := range append(corpus.Queries, corpus.Negatives...) {
		hits, mode, err := index.Search(context.Background(), access, q.Query, "", "", 40)
		if err != nil || mode != "hybrid" {
			t.Fatalf("retrieval: %s %v", mode, err)
		}
		docs := make([]Document, len(hits))
		for n, h := range hits {
			docs[n] = h.Document
		}
		started := time.Now()
		scores, err := reranker.Rerank(context.Background(), q.Query, docs)
		elapsed := float64(time.Since(started).Microseconds()) / 1000
		if err != nil {
			t.Fatalf("rerank query %d: %v", n, err)
		}
		split := "validation"
		if len(q.Expected) > 0 {
			if sha256.Sum256([]byte(q.Expected[0]))[0]%2 == 0 {
				split = "calibration"
			}
		} else if ((n-len(corpus.Queries))/2)%2 == 0 {
			split = "calibration"
		}
		row := rerankMeasurement{Case: q, Split: split, ElapsedMS: elapsed}
		for n, h := range hits {
			row.Candidates = append(row.Candidates, measuredCandidate{Name: h.Name, Ref: h.Ref, Base: h.Score, Score: scores[n]})
		}
		rows = append(rows, row)
		durations = append(durations, elapsed)
	}
	// Grid and split were fixed before scoring. Selection sees calibration only;
	// positive EN/TR Hit@1, Hit@5 and MRR must not fall below retrieval baseline.
	grid := []float64{0, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9}
	baselineCalibration := evaluateMeasurements(rows, "calibration", 5, 0, false)
	selected := 0.0
	found := false
	bestFP := math.MaxInt
	calibrationRows := []map[string]any{}
	for _, threshold := range grid {
		metrics := evaluateMeasurements(rows, "calibration", 20, threshold, true)
		calibrationRows = append(calibrationRows, map[string]any{"threshold": threshold, "metrics": metrics})
		if noRegression(metrics, baselineCalibration) && metrics["negative"].FalsePositives < bestFP {
			selected = threshold
			bestFP = metrics["negative"].FalsePositives
			found = true
		}
	}
	summaries := map[string]any{}
	for _, split := range []string{"calibration", "validation", "all"} {
		summaries[split] = map[string]any{"baseline": evaluateMeasurements(rows, split, 5, 0, false), "rerank10": evaluateMeasurements(rows, split, 10, 0, true), "rerank20": evaluateMeasurements(rows, split, 20, 0, true), "rerank40": evaluateMeasurements(rows, split, 40, 0, true), "calibrated20": evaluateMeasurements(rows, split, 20, selected, true)}
	}
	gate := found && noRegression(evaluateMeasurements(rows, "validation", 20, selected, true), evaluateMeasurements(rows, "validation", 5, 0, false))
	sort.Float64s(durations)
	percentile := func(p float64) float64 {
		return durations[min(len(durations)-1, int(math.Ceil(p*float64(len(durations))))-1)]
	}
	report := map[string]any{"embedding_model": embedder.Model, "rerank_model": reranker.Model, "score_transform": "sigmoid applied in the test provider; not probabilities", "catalog_size": len(corpus.Documents), "queries": len(rows), "selected_threshold": selected, "calibration_candidate_found": found, "validation_non_regression_gate": gate, "selection_candidate_window": 20, "threshold_grid": grid, "calibration_grid": calibrationRows, "summary": summaries, "rerank_latency_ms": map[string]float64{"p50": percentile(.5), "p95": percentile(.95), "p99": percentile(.99)}, "latency_scope": "40 candidates/request, sequential CPU model calls; 30s measurement timeout, product default 5s; not 100 TPS acceptance", "scope": "Known synthetic development corpus. Tool-paired language split fixed by SHA256 before scoring; negative translation pairs stay together. Validation is development partition, not independent institution holdout.", "results": rows}
	if path := os.Getenv("VMCP_TEST_RERANK_REPORT"); path != "" {
		data, _ := json.MarshalIndent(report, "", "  ")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("calibration candidate=%v threshold=%g development validation gate=%v; p95=%.2fms", found, selected, gate, percentile(.95))
	// Measurements are retained even when a candidate fails promotion. No automatic
	// production default change follows a synthetic quality result.
}

func TestRerankCalibratedPipeline(t *testing.T) {
	calibrationPath := os.Getenv("VMCP_TEST_RERANK_CALIBRATION")
	if calibrationPath == "" || os.Getenv("VMCP_TEST_RERANK_URL") == "" || os.Getenv("VMCP_TEST_EMBEDDING_URL") == "" {
		t.Skip("calibration and real endpoints required")
	}
	data, err := os.ReadFile(calibrationPath)
	if err != nil {
		t.Fatal(err)
	}
	var calibration struct {
		Floor      float64                                           `json:"selected_no_match_score"`
		Confidence float64                                           `json:"selected_confidence_score"`
		Found      bool                                              `json:"calibration_candidate_found"`
		Summary    map[string]map[string]map[string]relevanceMetrics `json:"summary"`
	}
	if err = json.Unmarshal(data, &calibration); err != nil {
		t.Fatal(err)
	}
	if !calibration.Found {
		t.Fatal("no calibration candidate passed; refusing to run an unselected configuration")
	}
	corpus := loadQualityCorpus(t)
	catalogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Catalog{Documents: corpus.Documents})
	}))
	defer catalogServer.Close()
	backend, _ := NewBackend(catalogServer.URL, strings.Repeat("s", 32))
	embedder, err := NewHTTPEmbedder(os.Getenv("VMCP_TEST_EMBEDDING_URL"), os.Getenv("VMCP_TEST_EMBEDDING_MODEL"), "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer embedder.Client.CloseIdleConnections()
	index := NewIndex(embedder)
	index.Refresh(context.Background(), backend)
	if index.Status().Error != "" {
		t.Fatal(index.Status())
	}
	reranker, err := NewHTTPReranker(os.Getenv("VMCP_TEST_RERANK_URL"), os.Getenv("VMCP_TEST_RERANK_MODEL"), "", false, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer reranker.Client.CloseIdleConnections()
	index.Reranker = reranker
	index.RerankCandidates = 20
	index.RerankFailurePolicy = "closed"
	index.RerankNoMatchScore = calibration.Floor
	index.RerankConfidenceScore = calibration.Confidence
	access := Access{Refs: map[string]string{}}
	for _, d := range corpus.Documents {
		access.Refs[d.Ref] = d.Hash()
	}
	refreshes := 0
	ctx := context.WithValue(context.Background(), accessRefreshKey{}, accessRefresh(func(context.Context) (Access, error) { refreshes++; return access, nil }))
	rows := []rerankMeasurement{}
	latency := []float64{}
	modes := map[string]int{}
	for n, q := range append(corpus.Queries, corpus.Negatives...) {
		started := time.Now()
		hits, mode, err := index.Search(ctx, access, q.Query, "", "", 5)
		elapsed := float64(time.Since(started).Microseconds()) / 1000
		if err != nil {
			t.Fatalf("live pipeline query %d: %v", n, err)
		}
		modes[mode]++
		latency = append(latency, elapsed)
		split := "validation"
		if len(q.Expected) > 0 {
			if sha256.Sum256([]byte(q.Expected[0]))[0]%2 == 0 {
				split = "calibration"
			}
		} else if ((n-len(corpus.Queries))/2)%2 == 0 {
			split = "calibration"
		}
		row := rerankMeasurement{Case: q, Split: split, ElapsedMS: elapsed}
		for _, h := range hits {
			if access.Refs[h.Ref] != h.Hash() {
				t.Fatal("unauthorized result")
			}
			row.Candidates = append(row.Candidates, measuredCandidate{Name: h.Name, Ref: h.Ref, Base: h.Score})
		}
		rows = append(rows, row)
		if (n+1)%15 == 0 {
			t.Logf("live calibrated pipeline %d/90", n+1)
		}
	}
	summary := map[string]map[string]relevanceMetrics{}
	for _, split := range []string{"calibration", "validation", "all"} {
		summary[split] = evaluateMeasurements(rows, split, 5, 0, false)
	}
	passed := noRegression(summary["validation"], calibration.Summary["validation"]["baseline"])
	sort.Float64s(latency)
	p := func(percentile float64) float64 {
		return latency[min(len(latency)-1, int(math.Ceil(percentile*float64(len(latency))))-1)]
	}
	report := map[string]any{"scope": "Live Index.Search + embedding + authorized rerank + query-level no-match and confidence guard on known synthetic development data. In-memory fresh-ACL callback; not a Kubernetes/100 TPS acceptance.", "embedding_model": embedder.Model, "rerank_model": reranker.Model, "catalog_size": len(corpus.Documents), "candidates": 20, "provider_timeout": "5s", "no_match_score": calibration.Floor, "confidence_score": calibration.Confidence, "failure_policy": "closed", "queries": len(rows), "pre_rerank_acl_refreshes": refreshes, "validation_non_regression_gate": passed, "summary": summary, "search_modes": modes, "search_latency_ms": map[string]float64{"p50": p(.5), "p95": p(.95), "p99": p(.99)}, "results": rows}
	if path := os.Getenv("VMCP_TEST_RERANK_PIPELINE_REPORT"); path != "" {
		out, _ := json.MarshalIndent(report, "", "  ")
		if err := os.WriteFile(path, out, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("live pipeline validation gate=%v p95=%.2fms modes=%v", passed, p(.95), modes)
	if !passed {
		t.Error("live calibrated pipeline regressed development validation metrics")
	}
	if refreshes != len(rows) {
		t.Fatal("missing pre-rerank authorization refresh")
	}
}
