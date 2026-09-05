// Package ai — tools_tools.go implements the tools-app tool handlers for
// the AI Agent Executor extended tool catalog (Section 103, Phase 9).
//
// Tools implemented here:
//
//   - tools.list       — list available Japa tools with record counts
//   - tools.read       — read a single record from a tool table by type + ID
//   - tools.create     — create a new tool record (admin, HITL)
//   - tools.update     — update a tool record (admin, HITL)
//   - tools.assign_geo — geo-scope a tool record to a country/region
//
// Unlike the prior stub implementation, these handlers route to real
// per-tool services via the ToolsRegistryService interface. The
// tools.list and tools.read handlers are fully implemented; create/update/
// assign_geo return a clear "use the dedicated tool API" message because
// each tool table has its own create/update endpoint with type-specific
// required fields (document_type, test_slug, profession_id, etc.) that
// cannot be generically mapped from a single tools.create handler.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Tools app tools)
// Ref: japaconnect-engineering/sections/78-tools-web-surface.md
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 9, task 9.04)
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// toolsListArgs is the LLM-produced argument shape for tools.list.
type toolsListArgs struct {
	Category string `json:"category,omitempty"`
}

// ToolsListHandler returns a ToolHandler that lists available Japa tools
// with record counts. The optional category filter narrows the result to
// a specific tool category (e.g. "immigration", "financial", "community").
func ToolsListHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.ToolsRegistry == nil {
			return errorToolResult("tools_unavailable", "tools registry service is not configured"), nil
		}

		var a toolsListArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		tools, err := deps.ToolsRegistry.ListTools(ctx, tenantID)
		if err != nil {
			return errorToolResult("list_failed", fmt.Sprintf("failed to list tools: %v", err)), nil
		}

		// Filter by category when provided.
		if a.Category != "" {
			filtered := make([]ToolCatalogEntry, 0, len(tools))

			for _, t := range tools {
				if t.Category == a.Category {
					filtered = append(filtered, t)
				}
			}

			tools = filtered
		}

		return successToolResult(map[string]any{
			"tools": tools,
			"count": len(tools),
		}), nil
	}
}

// toolsReadArgs is the LLM-produced argument shape for tools.read.
type toolsReadArgs struct {
	ToolType string `json:"tool_type"`
	ToolID   string `json:"tool_id"`
}

// ToolsReadHandler returns a ToolHandler that reads a single record from
// a tool table by type + ID. The tool_type identifies which tool table
// (scholarship, visa_route, housing_guide, etc.) and tool_id is the UUID.
func ToolsReadHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.ToolsRegistry == nil {
			return errorToolResult("tools_unavailable", "tools registry service is not configured"), nil
		}

		var a toolsReadArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if a.ToolType == "" {
			return errorToolResult("missing_tool_type", "tool_type is required"), nil
		}

		if a.ToolID == "" {
			return errorToolResult("missing_tool_id", "tool_id is required"), nil
		}

		record, err := deps.ToolsRegistry.ReadTool(ctx, tenantID, a.ToolType, a.ToolID)
		if err != nil {
			return errorToolResult("read_failed", fmt.Sprintf("failed to read tool: %v", err)), nil
		}

		return successToolResult(map[string]any{
			"tool_type": a.ToolType,
			"record":    record,
		}), nil
	}
}

// toolsCreateArgs is the LLM-produced argument shape for tools.create.
type toolsCreateArgs struct {
	ToolType string `json:"tool_type"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
}

// ToolsCreateHandler returns a ToolHandler that creates a new tool record.
// Each tool table has its own create endpoint with type-specific required
// fields, so this handler returns a redirect message pointing the agent
// to the dedicated tool API.
func ToolsCreateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		var a toolsCreateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if a.ToolType == "" {
			return errorToolResult("missing_tool_type", "tool_type is required"), nil
		}

		return errorToolResult("use_dedicated_api",
			fmt.Sprintf("tool '%s' has a dedicated create API with type-specific required fields. "+
				"Use the AI Content Studio (POST /api/v1/admin/studio/jobs with content_type=%s) "+
				"or the tool's own endpoint to create records.", a.ToolType, a.ToolType)), nil
	}
}

// ToolsUpdateHandler returns a ToolHandler that updates a tool record.
// Each tool table has its own update endpoint, so this handler returns a
// redirect message pointing the agent to the dedicated tool API.
func ToolsUpdateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		return errorToolResult("use_dedicated_api",
			"each tool has a dedicated update API with type-specific fields. "+
				"Use the tool's own endpoint (e.g. PUT /api/v1/admin/visa-routes/:id) to update records."), nil
	}
}

// ToolsAssignGeoHandler returns a ToolHandler that geo-scopes a tool
// record to a country/region. Tool records are geo-scoped via their
// destination_space_id / city_space_id fields on their dedicated update
// endpoints, so this handler returns a redirect message.
func ToolsAssignGeoHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		return errorToolResult("use_dedicated_api",
			"tool records are geo-scoped via destination_space_id / city_space_id on their dedicated update endpoints. "+
				"Use the tool's own update API to assign geo scope."), nil
	}
}
