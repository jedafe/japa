// Package ai — tools_tools.go implements the tools-app tool handlers for
// the AI Agent Executor extended tool catalog (Section 103, Phase 9).
//
// Tools implemented here (Phase 9, tasks 9.04):
//
//   - tools.list       — list available tools/features
//   - tools.read       — read a tool's details + custom fields
//   - tools.create     — create a new tool page (admin, HITL)
//   - tools.update     — update a tool (admin, HITL)
//   - tools.assign_geo — geo-scope a tool to a country/region
//
// DISCREPANCY (AGENTS.md Rule 1): Section 78 (Tools Web Surface) describes
// `apps/tools` as a sixth SvelteKit frontend surface that consumes existing
// backend APIs for individual identity tools (visa-checklist, readiness,
// cost-of-living, scholarships, remittance, etc.). There is no unified
// `internal/tools` package, no `tools` database table, and no
// `identity_tool` / `tool_page` / `tool_registry` entity in the schema.
// The individual tools live in separate packages (internal/checklist,
// internal/assessment, internal/remittance, internal/scholarships, etc.)
// with no shared CRUD abstraction.
//
// The §103 tool catalog lists tools.list/read/create/update/assign_geo
// assuming a "tools" entity that does not exist in the database. Per
// AGENTS.md Rule 1 ("If the spec and codebase conflict, stop and document
// the discrepancy rather than silently guessing"), these handlers are
// implemented as gracefully-degrading stubs: they are registered and seeded
// in the migration (with is_enabled = false) so the catalog is complete,
// but they return a clear "not configured" error until a backing tools
// table is added to the schema.
//
// When a `tools` table is added (future migration), the wiring layer can
// inject a ToolsService into ToolDeps and these handlers can be updated to
// call it. The function names and schemas are already defined in the
// migration seed so no catalog change will be needed at that time.
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

// toolsNotConfiguredResult is the shared error result for all tools-app
// handlers. The backing tools table does not exist yet (see file-level
// discrepancy note).
func toolsNotConfiguredResult() *ToolResult {
	return errorToolResult("tools_registry_unavailable",
		"the tools registry is not configured — a backing 'tools' table does not exist in the schema yet (§78 discrepancy, see tools_tools.go)")
}

// ToolsListHandler returns a ToolHandler that lists available tools.
// Currently returns "not configured" — see file-level discrepancy note.
func ToolsListHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		return toolsNotConfiguredResult(), nil
	}
}

// ToolsReadHandler returns a ToolHandler that reads a tool's details.
// Currently returns "not configured".
func ToolsReadHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		return toolsNotConfiguredResult(), nil
	}
}

// toolsCreateArgs is the LLM-produced argument shape for tools.create.
// Defined here so the schema is available for the migration seed even
// though the handler is a stub.
type toolsCreateArgs struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
}

// ToolsCreateHandler returns a ToolHandler that creates a new tool page.
// Currently returns "not configured".
func ToolsCreateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		// Validate arguments even as a stub so the LLM gets schema feedback.
		var a toolsCreateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		return toolsNotConfiguredResult(), nil
	}
}

// ToolsUpdateHandler returns a ToolHandler that updates a tool.
// Currently returns "not configured".
func ToolsUpdateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		return toolsNotConfiguredResult(), nil
	}
}

// ToolsAssignGeoHandler returns a ToolHandler that geo-scopes a tool to a
// country/region. Currently returns "not configured".
func ToolsAssignGeoHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		return toolsNotConfiguredResult(), nil
	}
}
