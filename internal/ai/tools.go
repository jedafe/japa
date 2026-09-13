// Package ai — tools.go defines the tool registry for AI-powered tools.
// // The tool registry manages configuration, enablement, and usage tracking
// for AI tools available to tenants. Each tool has a name, description,
// category, optional configuration schema, and an enable/disable toggle.
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
)

// AITool represents a registered AI tool in the system.
type AITool struct { //nolint:revive // stuttering type name preserved for API compatibility
	ID                 pgtype.UUID `json:"id"`
	TenantID           pgtype.UUID
	Name               string             `json:"name"`
	Description        string             `json:"description"`
	Category           string             `json:"category"`
	Config             json.RawMessage    `json:"config"`
	IsEnabled          bool               `json:"is_enabled"`
	RequiredPermission *string            `json:"required_permission,omitempty"`
	CreatedAt          pgtype.Timestamptz `json:"created_at"`
	UpdatedAt          pgtype.Timestamptz `json:"updated_at"`
}

// AIToolUsage tracks usage statistics for an AI tool.
type AIToolUsage struct { //nolint:revive // stuttering type name preserved for API compatibility
	ID         pgtype.UUID `json:"id"`
	TenantID   pgtype.UUID
	ToolID     pgtype.UUID        `json:"tool_id"`
	UserID     *pgtype.UUID       `json:"user_id,omitempty"`
	TokensUsed int32              `json:"tokens_used"`
	CostCents  int32              `json:"cost_cents"`
	CreatedAt  pgtype.Timestamptz `json:"created_at"`
}

// ToolRegistry provides CRUD operations for AI tool management.
type ToolRegistry struct {
	db *db.DB
}

// NewToolRegistry creates a ToolRegistry backed by the given database pool.
func NewToolRegistry(database *db.DB) *ToolRegistry {
	return &ToolRegistry{db: database}
}

// ListTools returns all AI tools for the given tenant.
func (r *ToolRegistry) ListTools(ctx context.Context, tenantID pgtype.UUID) ([]AITool, error) {
	rows, err := r.db.Reader.Query(ctx, `
		SELECT id, name, description, category, config, is_enabled,
		       required_permission, created_at, updated_at
		FROM ai_tools
		
		ORDER BY category, name
	`)
	if err != nil {
		return nil, fmt.Errorf("listing ai tools: %w", err)
	}
	defer rows.Close()

	var tools []AITool

	for rows.Next() {
		var t AITool
		if err := rows.Scan(
			&t.ID, &t.Name, &t.Description, &t.Category,
			&t.Config, &t.IsEnabled, &t.RequiredPermission,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning ai tool: %w", err)
		}

		tools = append(tools, t)
	}

	return tools, rows.Err()
}

// GetTool returns a single AI tool by ID.
func (r *ToolRegistry) GetTool(ctx context.Context, tenantID, id pgtype.UUID) (AITool, error) {
	var t AITool

	err := r.db.Reader.QueryRow(ctx, `
		SELECT id, name, description, category, config, is_enabled,
		       required_permission, created_at, updated_at
		FROM ai_tools
		WHERE id = $1
	`, id).Scan(
		&t.ID, &t.Name, &t.Description, &t.Category,
		&t.Config, &t.IsEnabled, &t.RequiredPermission,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return AITool{}, fmt.Errorf("fetching ai tool %s: %w", id, err)
	}

	return t, nil
}

// GetToolByName returns a single AI tool by name.
func (r *ToolRegistry) GetToolByName(ctx context.Context, tenantID pgtype.UUID, name string) (AITool, error) {
	var t AITool

	err := r.db.Reader.QueryRow(ctx, `
		SELECT id, name, description, category, config, is_enabled,
		       required_permission, created_at, updated_at
		FROM ai_tools
		WHERE name = $1
	`, name).Scan(
		&t.ID, &t.Name, &t.Description, &t.Category,
		&t.Config, &t.IsEnabled, &t.RequiredPermission,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return AITool{}, fmt.Errorf("fetching ai tool %s: %w", name, err)
	}

	return t, nil
}

// UpdateTool updates an existing AI tool's configuration, enablement, or description.
func (r *ToolRegistry) UpdateTool(ctx context.Context, tenantID, id pgtype.UUID, input UpdateToolInput) (AITool, error) {
	// Fetch current values for merge.
	current, err := r.GetTool(ctx, tenantID, id)
	if err != nil {
		return AITool{}, err
	}

	description := current.Description
	if input.Description != nil {
		description = *input.Description
	}

	config := current.Config
	if input.Config != nil {
		config = input.Config
	}

	isEnabled := current.IsEnabled
	if input.IsEnabled != nil {
		isEnabled = *input.IsEnabled
	}

	requiredPermission := current.RequiredPermission
	if input.RequiredPermission != nil {
		requiredPermission = input.RequiredPermission
	}

	var updated AITool

	err = r.db.Writer.QueryRow(ctx, `
		UPDATE ai_tools
		SET description = $2, config = $3, is_enabled = $4, required_permission = $5, updated_at = now()
		WHERE id = $1
		RETURNING id, name, description, category, config, is_enabled,
		          required_permission, created_at, updated_at
	`, id, description, config, isEnabled, requiredPermission).Scan(
		&updated.ID, &updated.Name, &updated.Description,
		&updated.Category, &updated.Config, &updated.IsEnabled,
		&updated.RequiredPermission, &updated.CreatedAt, &updated.UpdatedAt,
	)
	if err != nil {
		return AITool{}, fmt.Errorf("updating ai tool %s: %w", id, err)
	}

	return updated, nil
}

// DeleteTool removes an AI tool by ID.
func (r *ToolRegistry) DeleteTool(ctx context.Context, tenantID, id pgtype.UUID) error {
	tag, err := r.db.Writer.Exec(ctx, `
		DELETE FROM ai_tools
		WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("deleting ai tool %s: %w", id, err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("ai tool %s not found", id)
	}

	return nil
}

// UpdateToolInput is the input for updating an AI tool.
type UpdateToolInput struct {
	Description        *string         `json:"description,omitempty"`
	Config             json.RawMessage `json:"config,omitempty"`
	IsEnabled          *bool           `json:"is_enabled,omitempty"`
	RequiredPermission *string         `json:"required_permission,omitempty"`
}

// GetToolUsageStats returns aggregated usage statistics for a tool.
func (r *ToolRegistry) GetToolUsageStats(ctx context.Context, tenantID, toolID pgtype.UUID, since time.Time) (ToolUsageStats, error) {
	var stats ToolUsageStats

	err := r.db.Reader.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(tokens_used), 0) as total_tokens,
			COALESCE(SUM(cost_cents), 0) as total_cost_cents,
			COUNT(*) as total_calls,
			COUNT(DISTINCT user_id) as unique_users
		FROM ai_tool_usage
		WHERE tool_id = $1 AND created_at >= $2
	`, toolID, since).Scan(
		&stats.TotalTokens, &stats.TotalCostCents,
		&stats.TotalCalls, &stats.UniqueUsers,
	)
	if err != nil {
		return ToolUsageStats{}, fmt.Errorf("fetching tool usage stats: %w", err)
	}

	return stats, nil
}

// ToolUsageStats holds aggregated usage statistics for an AI tool.
type ToolUsageStats struct {
	TotalTokens    int64 `json:"total_tokens"`
	TotalCostCents int64 `json:"total_cost_cents"`
	TotalCalls     int64 `json:"total_calls"`
	UniqueUsers    int64 `json:"unique_users"`
}

// ListToolUsage returns recent usage records for a tool.
func (r *ToolRegistry) ListToolUsage(ctx context.Context, tenantID, toolID pgtype.UUID, limit, offset int32) ([]AIToolUsage, error) {
	rows, err := r.db.Reader.Query(ctx, `
		SELECT id, tool_id, user_id, tokens_used, cost_cents, created_at
		FROM ai_tool_usage
		WHERE tool_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, toolID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing tool usage: %w", err)
	}
	defer rows.Close()

	var usage []AIToolUsage

	for rows.Next() {
		var u AIToolUsage
		if err := rows.Scan(
			&u.ID, &u.ToolID, &u.UserID,
			&u.TokensUsed, &u.CostCents, &u.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning tool usage: %w", err)
		}

		usage = append(usage, u)
	}

	return usage, rows.Err()
}

// AllToolUsageStats holds per-tool aggregated usage for the admin dashboard.
type AllToolUsageStats struct {
	ToolID       string `json:"tool_id"`
	ToolName     string `json:"tool_name"`
	Invocations  int64  `json:"invocations"`
	TokensUsed   int64  `json:"tokens_used"`
	TotalCostUSD int64  `json:"total_cost_usd"`
}

// GetAllToolUsageStats returns per-tool usage aggregated across all tools for
// a tenant within the given period. Used by the admin /ai usage tab when no
// specific tool_id is selected.
func (r *ToolRegistry) GetAllToolUsageStats(ctx context.Context, tenantID pgtype.UUID, since time.Time) ([]AllToolUsageStats, error) {
	rows, err := r.db.Reader.Query(ctx, `
		SELECT
			t.id::text,
			t.name,
			COUNT(u.*) AS invocations,
			COALESCE(SUM(u.tokens_used), 0) AS tokens_used,
			COALESCE(SUM(u.cost_cents), 0) AS total_cost_cents
		FROM ai_tools t
		LEFT JOIN ai_tool_usage u ON u.tool_id = t.id AND u.created_at >= $1
		
		GROUP BY t.id, t.name
		ORDER BY invocations DESC
	`, tenantID, since)
	if err != nil {
		return nil, fmt.Errorf("fetching all tool usage stats: %w", err)
	}
	defer rows.Close()

	var stats []AllToolUsageStats

	for rows.Next() {
		var s AllToolUsageStats

		var totalCostCents int64
		if err := rows.Scan(&s.ToolID, &s.ToolName, &s.Invocations, &s.TokensUsed, &totalCostCents); err != nil {
			return nil, fmt.Errorf("scanning all tool usage stats: %w", err)
		}

		s.TotalCostUSD = totalCostCents / 100
		stats = append(stats, s)
	}

	return stats, rows.Err()
}

// RecordUsage records a single usage event for an AI tool.
func (r *ToolRegistry) RecordUsage(ctx context.Context, tenantID, toolID pgtype.UUID, userID *pgtype.UUID, tokensUsed, costCents int32) (pgtype.UUID, error) {
	var id pgtype.UUID

	err := r.db.Writer.QueryRow(ctx, `
		INSERT INTO ai_tool_usage (tool_id, user_id, tokens_used, cost_cents) VALUES ($1, $2, $3, $4)
		RETURNING id
	`, toolID, userID, tokensUsed, costCents).Scan(&id)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("recording tool usage: %w", err)
	}

	return id, nil
}

// EnsureTool registers a tool if it doesn't already exist for the tenant.
// Returns the tool ID (newly created or existing).
func (r *ToolRegistry) EnsureTool(ctx context.Context, tenantID pgtype.UUID, tool ToolDefinition) (pgtype.UUID, error) {
	var id pgtype.UUID

	err := r.db.Writer.QueryRow(ctx, `
		INSERT INTO ai_tools (name, description, category, config, is_enabled, required_permission) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (name) DO UPDATE
		SET description = EXCLUDED.description,
		    category = EXCLUDED.category,
		    updated_at = now()
		RETURNING id
	`, tool.Name, tool.Description, tool.Category, tool.Config, tool.IsEnabled, tool.RequiredPermission).Scan(&id)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("ensuring ai tool: %w", err)
	}

	return id, nil
}

// ToolDefinition is the input for creating or ensuring a tool exists.
type ToolDefinition struct {
	Name               string          `json:"name"`
	Description        string          `json:"description"`
	Category           string          `json:"category"`
	Config             json.RawMessage `json:"config,omitempty"`
	IsEnabled          bool            `json:"is_enabled"`
	RequiredPermission string          `json:"required_permission,omitempty"`
}
