// Package ai — tools.go defines the tool registry for AI-powered tools.
//
// The tool registry manages configuration, enablement, and usage tracking
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
	ID                 pgtype.UUID        `json:"id"`
	TenantID           pgtype.UUID        `json:"tenant_id"`
	Name               string             `json:"name"`
	Description        string             `json:"description"`
	Category           string             `json:"category"`
	Config             json.RawMessage    `json:"config"`
	IsEnabled          bool               `json:"is_enabled"`
	RequiredPermission string             `json:"required_permission,omitempty"`
	CreatedAt          pgtype.Timestamptz `json:"created_at"`
	UpdatedAt          pgtype.Timestamptz `json:"updated_at"`
}

// AIToolUsage tracks usage statistics for an AI tool.
type AIToolUsage struct { //nolint:revive // stuttering type name preserved for API compatibility
	ID         pgtype.UUID        `json:"id"`
	TenantID   pgtype.UUID        `json:"tenant_id"`
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
		SELECT id, tenant_id, name, description, category, config, is_enabled,
		       required_permission, created_at, updated_at
		FROM ai_tools
		WHERE tenant_id = $1
		ORDER BY category, name
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing ai tools: %w", err)
	}
	defer rows.Close()

	var tools []AITool

	for rows.Next() {
		var t AITool
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.Name, &t.Description, &t.Category,
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
		SELECT id, tenant_id, name, description, category, config, is_enabled,
		       required_permission, created_at, updated_at
		FROM ai_tools
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id).Scan(
		&t.ID, &t.TenantID, &t.Name, &t.Description, &t.Category,
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
		SELECT id, tenant_id, name, description, category, config, is_enabled,
		       required_permission, created_at, updated_at
		FROM ai_tools
		WHERE tenant_id = $1 AND name = $2
	`, tenantID, name).Scan(
		&t.ID, &t.TenantID, &t.Name, &t.Description, &t.Category,
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
		requiredPermission = *input.RequiredPermission
	}

	var updated AITool

	err = r.db.Writer.QueryRow(ctx, `
		UPDATE ai_tools
		SET description = $3, config = $4, is_enabled = $5, required_permission = $6, updated_at = now()
		WHERE tenant_id = $1 AND id = $2
		RETURNING id, tenant_id, name, description, category, config, is_enabled,
		          required_permission, created_at, updated_at
	`, tenantID, id, description, config, isEnabled, requiredPermission).Scan(
		&updated.ID, &updated.TenantID, &updated.Name, &updated.Description,
		&updated.Category, &updated.Config, &updated.IsEnabled,
		&updated.RequiredPermission, &updated.CreatedAt, &updated.UpdatedAt,
	)
	if err != nil {
		return AITool{}, fmt.Errorf("updating ai tool %s: %w", id, err)
	}

	return updated, nil
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
		WHERE tenant_id = $1 AND tool_id = $2 AND created_at >= $3
	`, tenantID, toolID, since).Scan(
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
		SELECT id, tenant_id, tool_id, user_id, tokens_used, cost_cents, created_at
		FROM ai_tool_usage
		WHERE tenant_id = $1 AND tool_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`, tenantID, toolID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing tool usage: %w", err)
	}
	defer rows.Close()

	var usage []AIToolUsage

	for rows.Next() {
		var u AIToolUsage
		if err := rows.Scan(
			&u.ID, &u.TenantID, &u.ToolID, &u.UserID,
			&u.TokensUsed, &u.CostCents, &u.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning tool usage: %w", err)
		}

		usage = append(usage, u)
	}

	return usage, rows.Err()
}

// RecordUsage records a single usage event for an AI tool.
func (r *ToolRegistry) RecordUsage(ctx context.Context, tenantID, toolID pgtype.UUID, userID *pgtype.UUID, tokensUsed, costCents int32) (pgtype.UUID, error) {
	var id pgtype.UUID

	err := r.db.Writer.QueryRow(ctx, `
		INSERT INTO ai_tool_usage (tenant_id, tool_id, user_id, tokens_used, cost_cents)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, tenantID, toolID, userID, tokensUsed, costCents).Scan(&id)
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
		INSERT INTO ai_tools (tenant_id, name, description, category, config, is_enabled, required_permission)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (tenant_id, name) DO UPDATE
		SET description = EXCLUDED.description,
		    category = EXCLUDED.category,
		    updated_at = now()
		RETURNING id
	`, tenantID, tool.Name, tool.Description, tool.Category, tool.Config,
		tool.IsEnabled, tool.RequiredPermission).Scan(&id)
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
