// Package ai — tools_jce.go implements the JCE refresh agent tool
// (Section 103, ADR-111).
//
// The comparison.refresh tool triggers a refresh of stale comparison values
// from registered data sources. The handler is a thin adapter over the
// ai.JCEService interface; the concrete implementation is provided by
// internal/jce.ToolAdapter.
//
// Ref: docs/superpowers/plans/2026-08-30-jce-subspec-4.md (Task 8)
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Comparison Engine)
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// Argument shape
// ---------------------------------------------------------------------------

// jceArgs is the LLM-produced argument shape for the comparison.refresh tool.
type jceArgs struct {
	ContentID  string `json:"content_id"`
	SourceKind string `json:"source_kind,omitempty"`
}

// ---------------------------------------------------------------------------
// comparison.refresh
// ---------------------------------------------------------------------------

// JCERefreshHandler returns a ToolHandler that triggers a refresh of stale
// comparison values from registered data sources.
func JCERefreshHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.JCE == nil {
			return errorToolResult("jce_unavailable", "JCE service is not configured"), nil
		}

		var a jceArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if a.ContentID == "" {
			return errorToolResult("missing_content_id", "content_id is required"), nil
		}

		contentID, err := scanUUID(a.ContentID)
		if err != nil {
			return errorToolResult("invalid_content_id", "content_id must be a valid UUID"), nil
		}

		// The refresh tool writes an ingestion job and enqueues a worker
		// task, so it requires the jce.refresh.trigger permission in
		// addition to the tool's top-level required_permission.
		if deps.Permission != nil && !deps.Permission.HasPermission(ctx, userID, "jce.refresh.trigger") {
			b, _ := json.Marshal(map[string]string{"error": "you don't have permission to trigger a refresh", "code": "unauthorized"})
			return &ToolResult{Status: ToolCallStatusUnauthorized, Content: string(b)}, nil
		}

		var sourceKind *string
		if a.SourceKind != "" {
			sourceKind = &a.SourceKind
		}

		result, err := deps.JCE.Refresh(ctx, contentID, userID, sourceKind)
		if err != nil {
			return errorToolResult("refresh_failed", err.Error()), nil
		}

		return successToolResult(result), nil
	}
}

// JCERefreshPrivacyTarget extracts the content target for the executor's
// privacy gate.
func JCERefreshPrivacyTarget(ctx context.Context, args json.RawMessage) (PrivacyEntityType, pgtype.UUID, error) {
	return jcePrivacyTarget(args)
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// jcePrivacyTarget extracts the content_id from JCE tool arguments so the
// executor's privacy gate can block the call when the backing content row is
// ai_exempt. It mirrors the pattern used by comparison.read.
func jcePrivacyTarget(args json.RawMessage) (PrivacyEntityType, pgtype.UUID, error) {
	var a jceArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return PrivacyEntityType(""), pgtype.UUID{}, nil
	}

	if a.ContentID == "" {
		return PrivacyEntityType(""), pgtype.UUID{}, nil
	}

	id, err := scanUUID(a.ContentID)
	if err != nil || !id.Valid {
		return PrivacyEntityType(""), pgtype.UUID{}, nil
	}

	return PrivacyEntityContent, id, nil
}
