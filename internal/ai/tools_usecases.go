// Package ai — tools_usecases.go implements dedicated per-use-case
// create/update/delete/assign-geo handlers for each tool table in the
// JapaConnect agent tool catalog (Section 103, Phase 10).
//
// Each handler is registered under <tool_type>.<action>, e.g.
// "visa_routes.create" or "housing_guides.update". The payload is forwarded
// to the ToolsRegistryService, which adapts the generic map[string]any
// arguments to the typed inputs of the per-tool service.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Tools app tools)
// Ref: docs/superpowers/specs/2026-08-24-dedicated-ai-tool-crud.md
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// useCaseUpdateArgs is the LLM-produced argument shape for .update,
// .delete, and .assign_geo handlers.
type useCaseUpdateArgs struct {
	ToolID  string         `json:"tool_id"`
	Payload map[string]any `json:"payload"`
}

// UseCaseCreateHandler returns a ToolHandler that creates a new record in
// the named tool table.
func UseCaseCreateHandler(deps *ToolDeps, toolType string) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.ToolsRegistry == nil {
			return errorToolResult("tools_unavailable", "tools registry service is not configured"), nil
		}

		payload, err := payloadFromArgs(args)
		if err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		record, err := deps.ToolsRegistry.CreateTool(ctx, tenantID, toolType, payload)
		if err != nil {
			return errorToolResult("create_failed", fmt.Sprintf("failed to create %s: %v", toolType, err)), nil
		}

		return successToolResult(map[string]any{
			"tool_type": toolType,
			"record":    record,
		}), nil
	}
}

// UseCaseUpdateHandler returns a ToolHandler that updates an existing record
// in the named tool table.
func UseCaseUpdateHandler(deps *ToolDeps, toolType string) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.ToolsRegistry == nil {
			return errorToolResult("tools_unavailable", "tools registry service is not configured"), nil
		}

		var a useCaseUpdateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.ToolID == "" {
			return errorToolResult("missing_tool_id", "tool_id is required"), nil
		}

		record, err := deps.ToolsRegistry.UpdateTool(ctx, tenantID, toolType, a.ToolID, a.Payload)
		if err != nil {
			return errorToolResult("update_failed", fmt.Sprintf("failed to update %s: %v", toolType, err)), nil
		}

		return successToolResult(map[string]any{
			"tool_type": toolType,
			"tool_id":   a.ToolID,
			"record":    record,
		}), nil
	}
}

// UseCaseDeleteHandler returns a ToolHandler that soft-deletes an existing
// record from the named tool table.
func UseCaseDeleteHandler(deps *ToolDeps, toolType string) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.ToolsRegistry == nil {
			return errorToolResult("tools_unavailable", "tools registry service is not configured"), nil
		}

		var a useCaseUpdateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.ToolID == "" {
			return errorToolResult("missing_tool_id", "tool_id is required"), nil
		}

		if err := deps.ToolsRegistry.DeleteTool(ctx, tenantID, toolType, a.ToolID); err != nil {
			return errorToolResult("delete_failed", fmt.Sprintf("failed to delete %s: %v", toolType, err)), nil
		}

		return successToolResult(map[string]any{
			"tool_type": toolType,
			"tool_id":   a.ToolID,
			"deleted":   true,
		}), nil
	}
}

// UseCaseAssignGeoHandler returns a ToolHandler that updates the geo-scoping
// fields of an existing record in the named tool table.
func UseCaseAssignGeoHandler(deps *ToolDeps, toolType string) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.ToolsRegistry == nil {
			return errorToolResult("tools_unavailable", "tools registry service is not configured"), nil
		}

		var a useCaseUpdateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.ToolID == "" {
			return errorToolResult("missing_tool_id", "tool_id is required"), nil
		}

		record, err := deps.ToolsRegistry.AssignGeo(ctx, tenantID, toolType, a.ToolID, a.Payload)
		if err != nil {
			return errorToolResult("assign_geo_failed", fmt.Sprintf("failed to assign geo for %s: %v", toolType, err)), nil
		}

		return successToolResult(map[string]any{
			"tool_type": toolType,
			"tool_id":   a.ToolID,
			"record":    record,
		}), nil
	}
}

// payloadFromArgs accepts either a top-level map of fields or an object with
// a "payload" key, returning the map that should be passed to the service.
func payloadFromArgs(args json.RawMessage) (map[string]any, error) {
	var top map[string]any
	if err := json.Unmarshal(args, &top); err != nil {
		return nil, err
	}
	if payload, ok := top["payload"]; ok {
		if m, ok := payload.(map[string]any); ok {
			return m, nil
		}
	}
	return top, nil
}
