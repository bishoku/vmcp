package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PGVectorStore implements VectorStore using PostgreSQL and the pgvector extension.
// It serves as the primary Alternative X to Qdrant, operating directly inside the
// existing vMCP PostgreSQL database without needing any additional microservices.
type PGVectorStore struct {
	pool      *pgxpool.Pool
	tableName string
	dimension int
}

// NewPGVectorStore creates a PostgreSQL pgvector store.
func NewPGVectorStore(pool *pgxpool.Pool, tableName string, dimension int) *PGVectorStore {
	if tableName == "" {
		tableName = "mcp_tool_embeddings"
	}
	if dimension <= 0 {
		dimension = 384
	}
	return &PGVectorStore{
		pool:      pool,
		tableName: tableName,
		dimension: dimension,
	}
}

func (s *PGVectorStore) Name() string {
	return "pgvector"
}

// Init creates vector extension, tools table, GIN role index, and HNSW cosine vector index.
func (s *PGVectorStore) Init(ctx context.Context) error {
	// 1. Enable pgvector extension
	if _, err := s.pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector;`); err != nil {
		return fmt.Errorf("enable pgvector extension failed: %w", err)
	}

	// 2. Create tools table with vector column
	createTableSQL := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
    id VARCHAR(128) PRIMARY KEY,
    server_id VARCHAR(64) NOT NULL,
    name VARCHAR(128) NOT NULL,
    domain VARCHAR(64) NOT NULL,
    description TEXT NOT NULL,
    input_schema JSONB NOT NULL,
    allowed_roles TEXT[] NOT NULL DEFAULT '{"admin"}',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    version INT NOT NULL DEFAULT 1,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    embedding vector(%d) NOT NULL
);`, s.tableName, s.dimension)

	if _, err := s.pool.Exec(ctx, createTableSQL); err != nil {
		return fmt.Errorf("create %s table failed: %w", s.tableName, err)
	}

	// 3. Create GIN index for role-based RBAC lookups
	idxRoleSQL := fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_roles ON %s USING GIN (allowed_roles);`, s.tableName, s.tableName)
	if _, err := s.pool.Exec(ctx, idxRoleSQL); err != nil {
		return fmt.Errorf("create roles index failed: %w", err)
	}

	// 4. Create index for active/domain pre-filtering
	idxActiveSQL := fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_active ON %s (is_active, domain);`, s.tableName, s.tableName)
	if _, err := s.pool.Exec(ctx, idxActiveSQL); err != nil {
		return fmt.Errorf("create active index failed: %w", err)
	}

	// 5. Create HNSW Cosine Index (m=16, ef_construction=100)
	idxHNSWSQL := fmt.Sprintf(`
CREATE INDEX IF NOT EXISTS idx_%s_hnsw ON %s 
USING hnsw (embedding vector_cosine_ops) 
WITH (m = 16, ef_construction = 100);`, s.tableName, s.tableName)

	if _, err := s.pool.Exec(ctx, idxHNSWSQL); err != nil {
		// Log or tolerate if table is empty or HNSW is already building
		return fmt.Errorf("create hnsw index failed: %w", err)
	}

	return nil
}

// Upsert writes or updates tool records and their dense vectors atomically.
func (s *PGVectorStore) Upsert(ctx context.Context, tools []ToolRecord, vectors [][]float32) error {
	if len(tools) == 0 || len(tools) != len(vectors) {
		return nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	upsertSQL := fmt.Sprintf(`
INSERT INTO %s (id, server_id, name, domain, description, input_schema, allowed_roles, is_active, version, updated_at, embedding)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::vector)
ON CONFLICT (id) DO UPDATE SET
    server_id = EXCLUDED.server_id,
    name = EXCLUDED.name,
    domain = EXCLUDED.domain,
    description = EXCLUDED.description,
    input_schema = EXCLUDED.input_schema,
    allowed_roles = EXCLUDED.allowed_roles,
    is_active = EXCLUDED.is_active,
    version = EXCLUDED.version,
    updated_at = EXCLUDED.updated_at,
    embedding = EXCLUDED.embedding;
`, s.tableName)

	for i, t := range tools {
		vecStr := formatVector(vectors[i])
		schemaJSON := t.InputSchema
		if len(schemaJSON) == 0 {
			schemaJSON = []byte("{}")
		}

		_, err := tx.Exec(ctx, upsertSQL,
			t.ID,
			t.ServerID,
			t.Name,
			t.Domain,
			t.Description,
			schemaJSON,
			t.AllowedRoles,
			t.IsActive,
			t.Version,
			t.UpdatedAt,
			vecStr,
		)
		if err != nil {
			return fmt.Errorf("upsert tool %s failed: %w", t.ID, err)
		}
	}

	return tx.Commit(ctx)
}

// Search performs cosine vector distance query (`1 - (embedding <=> $1)`) with SQL pre-filtering.
func (s *PGVectorStore) Search(ctx context.Context, query SearchQuery, queryVector []float32) ([]SearchResult, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 10
	}

	vecStr := formatVector(queryVector)

	// Build dynamic WHERE clause
	conditions := []string{"is_active = TRUE"}
	args := []any{vecStr}
	argIdx := 2

	if query.UserRole != "" {
		conditions = append(conditions, fmt.Sprintf("$%d = ANY(allowed_roles)", argIdx))
		args = append(args, query.UserRole)
		argIdx++
	}

	if query.Domain != "" {
		conditions = append(conditions, fmt.Sprintf("domain = $%d", argIdx))
		args = append(args, query.Domain)
		argIdx++
	}

	if query.ServerID != "" {
		conditions = append(conditions, fmt.Sprintf("server_id = $%d", argIdx))
		args = append(args, query.ServerID)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	querySQL := fmt.Sprintf(`
SELECT 
    id, server_id, name, domain, description, input_schema, allowed_roles, is_active, version, updated_at,
    1 - (embedding <=> $1::vector) AS score
FROM %s
WHERE %s
ORDER BY embedding <=> $1::vector
LIMIT $%d;
`, s.tableName, whereClause, argIdx)

	args = append(args, limit)

	rows, err := s.pool.Query(ctx, querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("pgvector search query failed: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var (
			t          ToolRecord
			schemaRaw  []byte
			score      float32
			rolesArray []string
		)
		if err := rows.Scan(
			&t.ID,
			&t.ServerID,
			&t.Name,
			&t.Domain,
			&t.Description,
			&schemaRaw,
			&rolesArray,
			&t.IsActive,
			&t.Version,
			&t.UpdatedAt,
			&score,
		); err != nil {
			return nil, fmt.Errorf("scan pgvector row failed: %w", err)
		}

		if query.MinScore > 0 && score < query.MinScore {
			continue
		}

		t.InputSchema = json.RawMessage(schemaRaw)
		t.AllowedRoles = rolesArray
		results = append(results, SearchResult{
			Tool:  t,
			Score: score,
		})
	}

	return results, rows.Err()
}

// Delete removes tools from PostgreSQL table by IDs.
func (s *PGVectorStore) Delete(ctx context.Context, toolIDs []string) error {
	if len(toolIDs) == 0 {
		return nil
	}
	delSQL := fmt.Sprintf("DELETE FROM %s WHERE id = ANY($1);", s.tableName)
	_, err := s.pool.Exec(ctx, delSQL, toolIDs)
	return err
}

// Count returns the row count in the vector table.
func (s *PGVectorStore) Count(ctx context.Context) (int64, error) {
	var count int64
	row := s.pool.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s;", s.tableName))
	err := row.Scan(&count)
	return count, err
}

func (s *PGVectorStore) Close() error {
	return nil
}

// formatVector formats float32 slice into pgvector text representation "[0.123,0.456,...]".
func formatVector(vec []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, v := range vec {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%f", v)
	}
	b.WriteByte(']')
	return b.String()
}
