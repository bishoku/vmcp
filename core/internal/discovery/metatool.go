package discovery

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bishoku/vmcp/core/internal/mcp"
)

// MetaToolName is the standardized tool name exposed to AI agents for discovering tools.
const MetaToolName = "search_tools"

// SearchToolsInputSchema describes the JSON-RPC input schema for search_tools.
var SearchToolsInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "Natural language description or goal of what you want to achieve (e.g. 'restart pod in k8s', 'fetch user invoice from billing')"
    },
    "domain": {
      "type": "string",
      "description": "Optional functional domain filter (e.g. 'devops', 'database', 'crm', 'billing')"
    },
    "limit": {
      "type": "integer",
      "description": "Maximum number of tools to return (default 5, maximum 20)",
      "default": 5
    }
  },
  "required": ["query"]
}`)

// ContentItem represents an MCP result content element.
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// CallToolResult represents standard MCP JSON-RPC tool invocation response.
type CallToolResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// GetSearchToolsMetaTool returns the MCP Tool definition for the discovery meta-tool.
func GetSearchToolsMetaTool() mcp.Tool {
	return mcp.Tool{
		Name:        MetaToolName,
		Description: "Semantically searches across 10,000+ enterprise MCP tools to find the most relevant tools for your specific task. Returns tool definitions, descriptions, and parameter schemas.",
		InputSchema: SearchToolsInputSchema,
	}
}

// SearchToolsParams represents parsed arguments for a search_tools execution.
type SearchToolsParams struct {
	Query  string `json:"query"`
	Domain string `json:"domain,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// ExecuteSearchMetaTool executes the search_tools invocation against DiscoveryService.
func ExecuteSearchMetaTool(ctx context.Context, svc *Service, userRole string, rawArgs json.RawMessage) (*CallToolResult, error) {
	var params SearchToolsParams
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &params); err != nil {
			return nil, fmt.Errorf("invalid search_tools arguments: %w", err)
		}
	}

	if params.Query == "" {
		return &CallToolResult{
			IsError: true,
			Content: []ContentItem{
				{
					Type: "text",
					Text: "Error: 'query' parameter is required for search_tools.",
				},
			},
		}, nil
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}

	q := SearchQuery{
		Query:    params.Query,
		UserRole: userRole,
		Domain:   params.Domain,
		Limit:    limit,
	}

	results, err := svc.SearchTools(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("tool discovery failed: %w", err)
	}

	if len(results) == 0 {
		return &CallToolResult{
			Content: []ContentItem{
				{
					Type: "text",
					Text: fmt.Sprintf("No relevant tools found matching query: %q", params.Query),
				},
			},
		}, nil
	}

	// Format discovered tools into rich structured summary
	type toolSummary struct {
		Name              string          `json:"name"`
		Domain            string          `json:"domain"`
		Score             float32         `json:"relevance_score"`
		DecisionRationale string          `json:"decision_rationale"`
		Description       string          `json:"description"`
		InputSchema       json.RawMessage `json:"input_schema"`
	}

	summaries := make([]toolSummary, len(results))
	for i, r := range results {
		rationale := fmt.Sprintf("Ranked #%d with cosine relevance score %.4f. Matched query intent in '%s' domain, permitted for role '%s'.",
			i+1, r.Score, r.Tool.Domain, userRole)
		summaries[i] = toolSummary{
			Name:              r.Tool.Name,
			Domain:            r.Tool.Domain,
			Score:             r.Score,
			DecisionRationale: rationale,
			Description:       r.Tool.Description,
			InputSchema:       r.Tool.InputSchema,
		}
	}

	outJSON, err := json.MarshalIndent(summaries, "", "  ")
	if err != nil {
		return nil, err
	}

	return &CallToolResult{
		Content: []ContentItem{
			{
				Type: "text",
				Text: fmt.Sprintf("Found %d relevant tools for query %q:\n\n%s", len(results), params.Query, string(outJSON)),
			},
		},
	}, nil
}
