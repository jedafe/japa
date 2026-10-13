// Package ai — tools_assist.go implements the §20 V1 in-editor assist
// operations as agent executor tools (migration 000847). Every handler
// decodes the LLM's arguments and delegates to the shared AssistRunner
// service (assist.go) — the exact same code path as POST /ai/assist/*, so
// the agent gets the REST ops' prompt construction, model/voice
// resolution, humanize merge, and usage logging for free.
//
// RBAC is enforced at the catalog layer: all six ai_tools rows carry
// required_permission='ai.editor.assist' — the same node the REST routes
// check — so the executor filters them out for unauthorized users before
// these handlers ever run.
//
// Ref: japaconnect-engineering/sections/20-ai-core.md (V1 Tool Catalog)
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Assist tools)
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// assistToolDeps returns the shared assist runner or an error result when
// the dependency is not wired (graceful degradation — never panic).
func assistToolDeps(deps *ToolDeps) (AssistRunner, *ToolResult) {
	if deps == nil || deps.Assist == nil {
		return nil, errorToolResult("assist_unavailable", "AI assist is not configured")
	}

	return deps.Assist, nil
}

// assistToolError maps an assist-op error onto an error ToolResult:
// typed *AssistError values keep their code/message so the LLM can react
// (e.g. retry with different args on validation_error); anything else is
// an opaque provider failure.
func assistToolError(err error) *ToolResult {
	var ae *AssistError
	if errors.As(err, &ae) {
		return errorToolResult(ae.Code, ae.Message)
	}

	return errorToolResult("assist_failed", "The AI assist call failed")
}

// assistTextResult shapes a text-producing assist response for the LLM:
// the extracted reply text plus the provider ref for traceability.
func assistTextResult(resp Response) *ToolResult {
	return successToolResult(map[string]any{
		"text":         ExtractReplyText(resp),
		"provider_ref": resp.ProviderRef,
	})
}

// AssistImproveHandler — content.improve.
func AssistImproveHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		assist, notConfigured := assistToolDeps(deps)
		if notConfigured != nil {
			return notConfigured, nil
		}

		var in AssistImproveInput
		if err := json.Unmarshal(args, &in); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		resp, err := assist.AssistImprove(ctx, userID, in)
		if err != nil {
			return assistToolError(err), nil
		}

		return assistTextResult(resp), nil
	}
}

// AssistSummarizeHandler — content.summarize.
func AssistSummarizeHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		assist, notConfigured := assistToolDeps(deps)
		if notConfigured != nil {
			return notConfigured, nil
		}

		var in AssistSummarizeInput
		if err := json.Unmarshal(args, &in); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		resp, err := assist.AssistSummarize(ctx, userID, in)
		if err != nil {
			return assistToolError(err), nil
		}

		return assistTextResult(resp), nil
	}
}

// AssistTranslateHandler — platform.translate_string.
func AssistTranslateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		assist, notConfigured := assistToolDeps(deps)
		if notConfigured != nil {
			return notConfigured, nil
		}

		var in AssistTranslateInput
		if err := json.Unmarshal(args, &in); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		resp, err := assist.AssistTranslate(ctx, userID, in)
		if err != nil {
			return assistToolError(err), nil
		}

		return assistTextResult(resp), nil
	}
}

// AssistSuggestTagsHandler — content.suggest_tags.
func AssistSuggestTagsHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		assist, notConfigured := assistToolDeps(deps)
		if notConfigured != nil {
			return notConfigured, nil
		}

		var in AssistTagsInput
		if err := json.Unmarshal(args, &in); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		resp, err := assist.AssistGenerateTags(ctx, userID, in)
		if err != nil {
			return assistToolError(err), nil
		}

		return assistTextResult(resp), nil
	}
}

// AssistGenerateOGHandler — content.generate_og.
func AssistGenerateOGHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		assist, notConfigured := assistToolDeps(deps)
		if notConfigured != nil {
			return notConfigured, nil
		}

		var in AssistOGInput
		if err := json.Unmarshal(args, &in); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		resp, err := assist.AssistGenerateOG(ctx, userID, in)
		if err != nil {
			return assistToolError(err), nil
		}

		return assistTextResult(resp), nil
	}
}

// AssistGenerateDraftHandler — content.generate_draft: text-only
// structured drafting (title + body_html + field_values). Distinct from
// content.create_draft, which persists a draft row and queues a HITL
// review.
func AssistGenerateDraftHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		assist, notConfigured := assistToolDeps(deps)
		if notConfigured != nil {
			return notConfigured, nil
		}

		var in DraftInput
		if err := json.Unmarshal(args, &in); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		result, err := assist.AssistDraft(ctx, userID, in)
		if err != nil {
			return assistToolError(err), nil
		}

		return successToolResult(result), nil
	}
}
