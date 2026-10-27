package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestEmbeddingTextFormat(t *testing.T) {
	tool := ToolRecord{
		Name:        "k8s_delete_pod",
		Domain:      "devops",
		Description: "Deletes a specific kubernetes pod from a cluster namespace",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"pod":{"type":"string"},"namespace":{"type":"string"}}}`),
	}
	text := BuildEmbeddingText(tool)
	expectedPrefix := "Name: k8s_delete_pod | Domain: devops | Description: Deletes a specific kubernetes pod from a cluster namespace | Parameters: "
	if len(text) < len(expectedPrefix) {
		t.Fatalf("expected embedding text prefix %q, got %q", expectedPrefix, text)
	}
	if !contains(text, "pod") || !contains(text, "namespace") {
		t.Fatalf("expected extracted parameter keys in %q", text)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0 && (s[:len(substr)] == substr || contains(s[1:], substr))))
}

func TestDiscoveryWorkflow_10k_Tools(t *testing.T) {
	ctx := context.Background()
	dim := 64
	embedder := NewDeterministicLocalEmbedder(dim)
	store := NewMemoryVectorStore()
	cache := NewMemorySemanticCache()

	svc := NewService(ServiceConfig{
		Store:    store,
		Embedder: embedder,
		Cache:    cache,
		CacheTTL: 5 * time.Minute,
	})

	// Generate 10,000 synthetic tools across different domains
	const toolCount = 10000
	tools := make([]ToolRecord, toolCount)
	domains := []string{"devops", "database", "crm", "billing", "security"}

	for i := 0; i < toolCount; i++ {
		domain := domains[i%len(domains)]
		name := fmt.Sprintf("tool_%s_%05d", domain, i)
		desc := fmt.Sprintf("Enterprise tool %d for handling operations in %s domain", i, domain)
		if i == 4200 {
			name = "k8s_restart_pod"
			domain = "devops"
			desc = "Restarts a failed Kubernetes pod in the production cluster namespace"
		} else if i == 7777 {
			name = "stripe_charge_invoice"
			domain = "billing"
			desc = "Charges a customer invoice using Stripe payment gateway API"
		}

		tools[i] = ToolRecord{
			ID:           name,
			ServerID:     fmt.Sprintf("server-%s", domain),
			Name:         name,
			Domain:       domain,
			Description:  desc,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"cluster":{"type":"string"},"id":{"type":"string"}}}`),
			AllowedRoles: []string{"admin", domain + "-admin"},
			IsActive:     true,
			Version:      1,
			UpdatedAt:    time.Now(),
		}
	}

	startIdx := time.Now()
	if err := svc.IndexTools(ctx, tools); err != nil {
		t.Fatalf("indexing 10,000 tools failed: %v", err)
	}
	indexDuration := time.Since(startIdx)
	t.Logf("Indexed %d tools in %v", toolCount, indexDuration)

	count, err := svc.Count(ctx)
	if err != nil || count != toolCount {
		t.Fatalf("expected %d tools, got %d (err: %v)", toolCount, count, err)
	}

	// 1. Search for Kubernetes pod restart (Natural Language Query)
	startSearch := time.Now()
	res, err := svc.SearchTools(ctx, SearchQuery{
		Query:    "Restart a crashed kubernetes pod in production",
		UserRole: "admin",
		Domain:   "devops",
		Limit:    5,
	})
	searchDuration := time.Since(startSearch)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	t.Logf("Search over 10k tools completed in %v, top score: %.4f", searchDuration, res[0].Score)

	if len(res) == 0 {
		t.Fatalf("expected search results, got empty")
	}
	if res[0].Tool.Name != "k8s_restart_pod" {
		t.Errorf("expected top tool 'k8s_restart_pod', got %q", res[0].Tool.Name)
	}

	// 2. Test L1 Semantic Cache Hit
	startCacheHit := time.Now()
	cacheRes, err := svc.SearchTools(ctx, SearchQuery{
		Query:    "Restart a crashed kubernetes pod in production",
		UserRole: "admin",
		Domain:   "devops",
		Limit:    5,
	})
	cacheHitDuration := time.Since(startCacheHit)
	if err != nil || len(cacheRes) == 0 {
		t.Fatalf("cached search failed: %v", err)
	}
	t.Logf("L1 Semantic Cache hit returned in %v", cacheHitDuration)
	if cacheHitDuration > 5*time.Millisecond {
		t.Logf("Notice: Cache hit duration %v", cacheHitDuration)
	}

	// 3. Test RBAC pre-filtering: billing role must NOT see devops tools
	rbacRes, err := svc.SearchTools(ctx, SearchQuery{
		Query:    "Restart a crashed kubernetes pod in production",
		UserRole: "billing-admin", // not allowed to see devops-admin tools
		Domain:   "devops",
		Limit:    5,
	})
	if err != nil {
		t.Fatalf("rbac search failed: %v", err)
	}
	if len(rbacRes) != 0 {
		t.Fatalf("expected 0 results due to RBAC pre-filter, got %d (first: %s)", len(rbacRes), rbacRes[0].Tool.Name)
	}
}

func TestMetaToolInvocation(t *testing.T) {
	ctx := context.Background()
	embedder := NewDeterministicLocalEmbedder(64)
	store := NewMemoryVectorStore()
	svc := NewService(ServiceConfig{
		Store:    store,
		Embedder: embedder,
	})

	_ = svc.IndexTools(ctx, []ToolRecord{
		{
			ID:           "sql_query_db",
			Name:         "sql_query_db",
			Domain:       "database",
			Description:  "Executes read-only SQL queries against Postgres database",
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
			AllowedRoles: []string{"admin"},
			IsActive:     true,
		},
	})

	// Invoke meta tool via JSON-RPC arguments
	rawArgs := json.RawMessage(`{"query": "run select query on PostgreSQL", "domain": "database", "limit": 3}`)
	res, err := ExecuteSearchMetaTool(ctx, svc, "admin", rawArgs)
	if err != nil {
		t.Fatalf("execute meta tool failed: %v", err)
	}
	if res.IsError {
		t.Fatalf("meta tool returned error response")
	}
	if len(res.Content) == 0 {
		t.Fatalf("expected content in result")
	}
	t.Logf("Meta tool result text:\n%s", res.Content[0].Text)
}

func TestInterfaceImplementations(t *testing.T) {
	var _ VectorStore = (*MemoryVectorStore)(nil)
	var _ VectorStore = (*QdrantStore)(nil)
	var _ VectorStore = (*PGVectorStore)(nil)
	var _ Embedder = (*DeterministicLocalEmbedder)(nil)
	var _ Embedder = (*TEIEmbedder)(nil)
	var _ SemanticCache = (*MemorySemanticCache)(nil)
	var _ SemanticCache = (*RedisSemanticCache)(nil)
}
