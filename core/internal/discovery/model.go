package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bishoku/vmcp/core/internal/mcp"
)

// ToolRecord represents an indexed MCP tool with rich metadata for vector search and RBAC.
type ToolRecord struct {
	ID           string          `json:"id"`            // Unique tool ID, e.g., "server_id__tool_name"
	ServerID     string          `json:"server_id"`     // Upstream server ID
	Name         string          `json:"name"`          // Tool name
	Domain       string          `json:"domain"`        // Domain/category, e.g., "devops", "database", "crm"
	Description  string          `json:"description"`   // Functional description
	InputSchema  json.RawMessage `json:"input_schema"`  // JSON schema for parameters
	AllowedRoles []string        `json:"allowed_roles"` // RBAC roles allowed to discover/invoke this tool
	IsActive     bool            `json:"is_active"`     // Health status flag (true = active, false = offline)
	Version      int             `json:"version"`       // Schema version
	UpdatedAt    time.Time       `json:"updated_at"`    // Last update timestamp
}

// ToMCPTool converts the record to standard MCP Tool structure.
func (t ToolRecord) ToMCPTool() mcp.Tool {
	return mcp.Tool{
		Name:        t.Name,
		Description: t.Description,
		InputSchema: t.InputSchema,
	}
}

// SearchQuery represents a semantically routed search request.
type SearchQuery struct {
	Query    string   `json:"query"`               // Natural language query or prompt intent
	UserRole string   `json:"user_role"`           // Role of the caller (for RBAC pre-filtering)
	Domain   string   `json:"domain,omitempty"`    // Optional domain filter (e.g., "devops")
	ServerID string   `json:"server_id,omitempty"` // Optional upstream server filter
	Limit    int      `json:"limit"`               // Max number of tools to return (Top-K)
	MinScore float32  `json:"min_score,omitempty"` // Cosine similarity cutoff (e.g., 0.65)
}

// SearchResult represents a matched tool with its semantic similarity score.
type SearchResult struct {
	Tool  ToolRecord `json:"tool"`
	Score float32    `json:"score"` // Cosine similarity between 0.0 and 1.0
}

// BuildEmbeddingText formats tool information into a dense representation for the embedding model,
// as defined in TECHNICAL-SYSTEM-ARCHITECTURE.md.
func BuildEmbeddingText(tool ToolRecord) string {
	paramKeys := ExtractParameterKeys(tool.InputSchema)
	domain := tool.Domain
	if domain == "" {
		domain = "general"
	}
	return fmt.Sprintf("Name: %s | Domain: %s | Description: %s | Parameters: %s",
		tool.Name, domain, tool.Description, strings.Join(paramKeys, ", "))
}

// ExtractParameterKeys extracts top-level property names from JSON schema.
func ExtractParameterKeys(schema json.RawMessage) []string {
	if len(schema) == 0 {
		return nil
	}
	var doc struct {
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(schema, &doc); err != nil || doc.Properties == nil {
		return nil
	}
	keys := make([]string, 0, len(doc.Properties))
	for k := range doc.Properties {
		keys = append(keys, k)
	}
	return keys
}

// VectorStore is the pluggable storage interface for semantic tool discovery.
// It allows substituting Qdrant with Alternative X (e.g., PostgreSQL with pgvector) or Memory.
type VectorStore interface {
	// Name returns the provider identifier ("qdrant", "pgvector", "memory").
	Name() string
	// Init initializes collections/tables and indices (HNSW, Cosine metric, quantization).
	Init(ctx context.Context) error
	// Upsert writes or updates tools and their corresponding dense vectors.
	Upsert(ctx context.Context, tools []ToolRecord, vectors [][]float32) error
	// Search queries the vector store with pre-filtering on RBAC and active status.
	Search(ctx context.Context, query SearchQuery, queryVector []float32) ([]SearchResult, error)
	// Delete removes tool records and embeddings by their IDs.
	Delete(ctx context.Context, toolIDs []string) error
	// Count returns the total number of indexed vectors.
	Count(ctx context.Context) (int64, error)
	// Close releases any persistent client connections.
	Close() error
}

// Embedder generates vector embeddings from text strings.
type Embedder interface {
	// Dimension returns the vector dimensionality (e.g. 384 for bge-small).
	Dimension() int
	// Embed batches texts into float32 embedding vectors.
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
