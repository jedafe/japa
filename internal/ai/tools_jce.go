// Package ai — tools_jce.go implements the JCE Sub-spec 4 AI advisor and
// refresh agent tools (Section 103, ADR-111).
//
// These tools extend the Phase 7 comparison catalog with natural-language
// analysis, ranking, explanation, recommendations, and stale-value refresh.
// Each handler is a thin adapter over the ai.JCEService interface; the
// concrete implementation is provided by internal/jce.ToolAdapter.
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

// jceArgs is the common LLM-produced argument shape for all five JCE tools.
// Optional fields are empty when the tool does not use them.
type jceArgs struct {
	ContentID  string `json:"content_id"`
	ProfileID  string `json:"profile_id,omitempty"`
	ItemID     string `json:"item_id,omitempty"`
	FactorKey  string `json:"factor_key,omitempty"`
	SourceKind string `json:"source_kind,omitempty"`
	Question   string `json:"question,omitempty"`
}

// ---------------------------------------------------------------------------
// comparison.analyze
// ---------------------------------------------------------------------------

// JCEAnalyzeHandler returns a ToolHandler that answers a natural-language
// question about a comparison and profile.
func JCEAnalyzeHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
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

		profileID, err := optionalUUID(a.ProfileID)
		if err != nil {
			return errorToolResult("invalid_profile_id", "profile_id is not a valid UUID"), nil
		}

		result, err := deps.JCE.Analyze(ctx, contentID, tenantID, userID, profileID, a.Question)
		if err != nil {
			return errorToolResult("advisor_failed", err.Error()), nil
		}

		return successToolResult(result), nil
	}
}

// JCEAnalyzePrivacyTarget extracts the content target for the executor's
// privacy gate.
func JCEAnalyzePrivacyTarget(ctx context.Context, args json.RawMessage, tenantID pgtype.UUID) (PrivacyEntityType, pgtype.UUID, error) {
	return jcePrivacyTarget(args)
}

// ---------------------------------------------------------------------------
// comparison.rank
// ---------------------------------------------------------------------------

// JCERankHandler returns a ToolHandler that ranks comparison items with
// reasoning and confidence.
func JCERankHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
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

		profileID, err := optionalUUID(a.ProfileID)
		if err != nil {
			return errorToolResult("invalid_profile_id", "profile_id is not a valid UUID"), nil
		}

		result, err := deps.JCE.Rank(ctx, contentID, tenantID, userID, profileID)
		if err != nil {
			return errorToolResult("advisor_failed", err.Error()), nil
		}

		return successToolResult(result), nil
	}
}

// JCERankPrivacyTarget extracts the content target for the executor's
// privacy gate.
func JCERankPrivacyTarget(ctx context.Context, args json.RawMessage, tenantID pgtype.UUID) (PrivacyEntityType, pgtype.UUID, error) {
	return jcePrivacyTarget(args)
}

// ---------------------------------------------------------------------------
// comparison.explain
// ---------------------------------------------------------------------------

// JCEExplainHandler returns a ToolHandler that explains a single score or
// recommendation for a comparison item and factor.
func JCEExplainHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
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

		if a.ItemID == "" {
			return errorToolResult("missing_item_id", "item_id is required"), nil
		}

		contentID, err := scanUUID(a.ContentID)
		if err != nil {
			return errorToolResult("invalid_content_id", "content_id must be a valid UUID"), nil
		}

		itemID, err := scanUUID(a.ItemID)
		if err != nil {
			return errorToolResult("invalid_item_id", "item_id must be a valid UUID"), nil
		}

		profileID, err := optionalUUID(a.ProfileID)
		if err != nil {
			return errorToolResult("invalid_profile_id", "profile_id is not a valid UUID"), nil
		}

		result, err := deps.JCE.Explain(ctx, contentID, itemID, tenantID, userID, profileID, a.FactorKey, a.Question)
		if err != nil {
			return errorToolResult("advisor_failed", err.Error()), nil
		}

		return successToolResult(result), nil
	}
}

// JCEExplainPrivacyTarget extracts the content target for the executor's
// privacy gate.
func JCEExplainPrivacyTarget(ctx context.Context, args json.RawMessage, tenantID pgtype.UUID) (PrivacyEntityType, pgtype.UUID, error) {
	return jcePrivacyTarget(args)
}

// ---------------------------------------------------------------------------
// comparison.recommend_next_step
// ---------------------------------------------------------------------------

// JCERecommendHandler returns a ToolHandler that returns the top next actions
// and recommendations for a comparison and profile.
func JCERecommendHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
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

		profileID, err := optionalUUID(a.ProfileID)
		if err != nil {
			return errorToolResult("invalid_profile_id", "profile_id is not a valid UUID"), nil
		}

		result, err := deps.JCE.Recommend(ctx, contentID, tenantID, userID, profileID)
		if err != nil {
			return errorToolResult("advisor_failed", err.Error()), nil
		}

		return successToolResult(result), nil
	}
}

// JCERecommendPrivacyTarget extracts the content target for the executor's
// privacy gate.
func JCERecommendPrivacyTarget(ctx context.Context, args json.RawMessage, tenantID pgtype.UUID) (PrivacyEntityType, pgtype.UUID, error) {
	return jcePrivacyTarget(args)
}

// ---------------------------------------------------------------------------
// comparison.refresh
// ---------------------------------------------------------------------------

// JCERefreshHandler returns a ToolHandler that triggers a refresh of stale
// comparison values from registered data sources.
func JCERefreshHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
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
		if deps.Permission != nil && !deps.Permission.HasPermission(ctx, userID, tenantID, "jce.refresh.trigger") {
			b, _ := json.Marshal(map[string]string{"error": "you don't have permission to trigger a refresh", "code": "unauthorized"})
			return &ToolResult{Status: ToolCallStatusUnauthorized, Content: string(b)}, nil
		}

		var sourceKind *string
		if a.SourceKind != "" {
			sourceKind = &a.SourceKind
		}

		result, err := deps.JCE.Refresh(ctx, contentID, tenantID, userID, sourceKind)
		if err != nil {
			return errorToolResult("refresh_failed", err.Error()), nil
		}

		return successToolResult(result), nil
	}
}

// JCERefreshPrivacyTarget extracts the content target for the executor's
// privacy gate.
func JCERefreshPrivacyTarget(ctx context.Context, args json.RawMessage, tenantID pgtype.UUID) (PrivacyEntityType, pgtype.UUID, error) {
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
