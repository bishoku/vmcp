package discovery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// QdrantStore implements VectorStore backed by Qdrant Vector Database.
type QdrantStore struct {
	endpoint       string
	collectionName string
	dimension      int
	apiKey         string
	client         *http.Client
}

// QdrantConfig holds setup options for Qdrant.
type QdrantConfig struct {
	Endpoint       string // e.g. "http://qdrant:6333"
	CollectionName string // e.g. "mcp_tools"
	Dimension      int    // e.g. 384
	APIKey         string // optional API key
}

// NewQdrantStore creates a new Qdrant vector store adapter.
func NewQdrantStore(cfg QdrantConfig) *QdrantStore {
	if cfg.CollectionName == "" {
		cfg.CollectionName = "mcp_tools"
	}
	if cfg.Dimension <= 0 {
		cfg.Dimension = 384
	}
	tr := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 50,
		IdleConnTimeout:     90 * time.Second,
	}
	return &QdrantStore{
		endpoint:       strings.TrimRight(cfg.Endpoint, "/"),
		collectionName: cfg.CollectionName,
		dimension:      cfg.Dimension,
		apiKey:         cfg.APIKey,
		client:         &http.Client{Transport: tr, Timeout: 15 * time.Second},
	}
}

func (q *QdrantStore) Name() string {
	return "qdrant"
}

// Init creates the Qdrant collection with HNSW parameters and Scalar Quantization (int8),
// and creates payload keyword indexes for allowed_roles, domain, and is_active.
func (q *QdrantStore) Init(ctx context.Context) error {
	// Check if collection exists
	req, err := q.newRequest(ctx, http.MethodGet, fmt.Sprintf("/collections/%s", q.collectionName), nil)
	if err != nil {
		return err
	}
	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("qdrant connect error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil // Collection already initialized
	}

	// Collection creation specification adhering to TECHNICAL-SYSTEM-ARCHITECTURE.md
	// - HNSW: m: 16, ef_construct: 100
	// - Quantization: int8 scalar quantization for 75% memory reduction
	// - Distance: Cosine
	schema := map[string]any{
		"vectors": map[string]any{
			"size":     q.dimension,
			"distance": "Cosine",
		},
		"hnsw_config": map[string]any{
			"m":            16,
			"ef_construct": 100,
		},
		"quantization_config": map[string]any{
			"scalar": map[string]any{
				"type":   "int8",
				"always_ram": true,
			},
		},
	}

	createReq, err := q.newRequest(ctx, http.MethodPut, fmt.Sprintf("/collections/%s", q.collectionName), schema)
	if err != nil {
		return err
	}
	createResp, err := q.client.Do(createReq)
	if err != nil {
		return fmt.Errorf("create qdrant collection failed: %w", err)
	}
	defer createResp.Body.Close()

	if createResp.StatusCode != http.StatusOK && createResp.StatusCode != http.StatusCreated {
		return fmt.Errorf("qdrant collection creation returned %d", createResp.StatusCode)
	}

	// Create Payload Indexes for fast pre-filtering without disk hits
	payloadIndexes := []struct {
		field  string
		schema string
	}{
		{"allowed_roles", "keyword"},
		{"domain", "keyword"},
		{"server_id", "keyword"},
		{"is_active", "bool"},
	}

	for _, idx := range payloadIndexes {
		idxBody := map[string]any{
			"field_name":   idx.field,
			"field_schema": idx.schema,
		}
		pReq, err := q.newRequest(ctx, http.MethodPut, fmt.Sprintf("/collections/%s/index", q.collectionName), idxBody)
		if err == nil {
			if pResp, err := q.client.Do(pReq); err == nil {
				pResp.Body.Close()
			}
		}
	}

	return nil
}

// Upsert uploads tools with their vectors and indexed payloads to Qdrant.
func (q *QdrantStore) Upsert(ctx context.Context, tools []ToolRecord, vectors [][]float32) error {
	if len(tools) == 0 || len(tools) != len(vectors) {
		return nil
	}

	points := make([]map[string]any, len(tools))
	for i, t := range tools {
		pointUUID := deterministicUUID(t.ID)
		points[i] = map[string]any{
			"id":     pointUUID,
			"vector": vectors[i],
			"payload": map[string]any{
				"tool_id":       t.ID,
				"server_id":     t.ServerID,
				"name":          t.Name,
				"domain":        t.Domain,
				"description":   t.Description,
				"input_schema":  string(t.InputSchema),
				"allowed_roles": t.AllowedRoles,
				"is_active":     t.IsActive,
				"version":       t.Version,
				"updated_at":    t.UpdatedAt.Unix(),
			},
		}
	}

	reqBody := map[string]any{
		"points": points,
	}

	req, err := q.newRequest(ctx, http.MethodPut, fmt.Sprintf("/collections/%s/points?wait=true", q.collectionName), reqBody)
	if err != nil {
		return err
	}

	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("upsert points failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upsert returned status %d", resp.StatusCode)
	}
	return nil
}

// Search executes HNSW approximate vector search with RBAC pre-filtering.
func (q *QdrantStore) Search(ctx context.Context, query SearchQuery, queryVector []float32) ([]SearchResult, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 10
	}

	// Construct Qdrant filter condition
	mustConditions := []map[string]any{
		{
			"key": "is_active",
			"match": map[string]any{
				"value": true,
			},
		},
	}

	if query.UserRole != "" {
		mustConditions = append(mustConditions, map[string]any{
			"key": "allowed_roles",
			"match": map[string]any{
				"value": query.UserRole,
			},
		})
	}

	if query.Domain != "" {
		mustConditions = append(mustConditions, map[string]any{
			"key": "domain",
			"match": map[string]any{
				"value": query.Domain,
			},
		})
	}

	if query.ServerID != "" {
		mustConditions = append(mustConditions, map[string]any{
			"key": "server_id",
			"match": map[string]any{
				"value": query.ServerID,
			},
		})
	}

	searchBody := map[string]any{
		"vector":       queryVector,
		"limit":        limit,
		"with_payload": true,
		"params": map[string]any{
			"hnsw_ef": 64, // ef_search: 64 for sub-millisecond query
			"exact":   false,
		},
		"filter": map[string]any{
			"must": mustConditions,
		},
	}

	if query.MinScore > 0 {
		searchBody["score_threshold"] = query.MinScore
	}

	req, err := q.newRequest(ctx, http.MethodPost, fmt.Sprintf("/collections/%s/points/search", q.collectionName), searchBody)
	if err != nil {
		return nil, err
	}

	resp, err := q.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qdrant search call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qdrant search returned status %d", resp.StatusCode)
	}

	var responseEnvelope struct {
		Result []struct {
			Score   float32        `json:"score"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&responseEnvelope); err != nil {
		return nil, fmt.Errorf("decode qdrant response: %w", err)
	}

	results := make([]SearchResult, 0, len(responseEnvelope.Result))
	for _, item := range responseEnvelope.Result {
		p := item.Payload
		var inputSchema json.RawMessage
		if s, ok := p["input_schema"].(string); ok {
			inputSchema = json.RawMessage(s)
		}

		roles := make([]string, 0)
		if rRaw, ok := p["allowed_roles"].([]any); ok {
			for _, r := range rRaw {
				if rStr, ok := r.(string); ok {
					roles = append(roles, rStr)
				}
			}
		}

		tool := ToolRecord{
			ID:           getString(p, "tool_id"),
			ServerID:     getString(p, "server_id"),
			Name:         getString(p, "name"),
			Domain:       getString(p, "domain"),
			Description:  getString(p, "description"),
			InputSchema:  inputSchema,
			AllowedRoles: roles,
			IsActive:     getBool(p, "is_active"),
		}
		results = append(results, SearchResult{
			Tool:  tool,
			Score: item.Score,
		})
	}

	return results, nil
}

// Delete removes vectors by tool IDs.
func (q *QdrantStore) Delete(ctx context.Context, toolIDs []string) error {
	if len(toolIDs) == 0 {
		return nil
	}
	pointUUIDs := make([]string, len(toolIDs))
	for i, id := range toolIDs {
		pointUUIDs[i] = deterministicUUID(id)
	}

	body := map[string]any{
		"points": pointUUIDs,
	}

	req, err := q.newRequest(ctx, http.MethodPost, fmt.Sprintf("/collections/%s/points/delete?wait=true", q.collectionName), body)
	if err != nil {
		return err
	}
	resp, err := q.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// Count returns the number of points in collection.
func (q *QdrantStore) Count(ctx context.Context) (int64, error) {
	req, err := q.newRequest(ctx, http.MethodPost, fmt.Sprintf("/collections/%s/points/count", q.collectionName), map[string]any{})
	if err != nil {
		return 0, err
	}
	resp, err := q.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var envelope struct {
		Result struct {
			Count int64 `json:"count"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return 0, err
	}
	return envelope.Result.Count, nil
}

func (q *QdrantStore) Close() error {
	return nil
}

func (q *QdrantStore) newRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, q.endpoint+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if q.apiKey != "" {
		req.Header.Set("api-key", q.apiKey)
	}
	return req, nil
}

func getString(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

func getBool(m map[string]any, k string) bool {
	if v, ok := m[k].(bool); ok {
		return v
	}
	return false
}

// deterministicUUID creates a reproducible UUID from an arbitrary tool ID string.
func deterministicUUID(id string) string {
	h := sha256.Sum256([]byte(id))
	u, err := uuid.FromBytes(h[:16])
	if err != nil {
		return uuid.New().String()
	}
	return u.String()
}
