// Package ai — tools_intelligence.go implements the agent tools that drive
// the on-demand intelligence → content pipeline (§75, ADR-076 Amendment 2).
//
// Tools implemented here:
//
//   - source.crawl_now              enqueue intel:fetch_source for a single source
//   - record.update_now             resolve a record or source and enqueue fetch
//   - content.regenerate_from_source enqueue content:regenerate_from_source
//
// All handlers validate the underlying entity before enqueueing and return
// a queued task reference to the agent. Tenant scoping is enforced by the
// executor's RBAC gate and by the gen.Queries predicates.
//
// Ref: docs/superpowers/specs/2026-08-19-on-demand-intel-to-content.md
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Tool Catalog Expansion)
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/worker"
)

// ---------------------------------------------------------------------------
// source.crawl_now
// ---------------------------------------------------------------------------

type sourceCrawlNowArgs struct {
	SourceID string `json:"source_id"`
}

// SourceCrawlNowHandler returns a ToolHandler that enqueues an immediate
// crawl for a single intel source. The source must exist and be enabled.
func SourceCrawlNowHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.TaskEnqueuer == nil {
			return errorToolResult("task_enqueuer_unavailable", "task enqueuer is not configured"), nil
		}

		var a sourceCrawlNowArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.SourceID == "" {
			return errorToolResult("missing_source_id", "source_id is required"), nil
		}

		sourceID, err := scanUUID(a.SourceID)
		if err != nil {
			return errorToolResult("invalid_source_id", "source_id is not a valid UUID"), nil
		}

		if deps.IntelQuerier != nil {
			src, err := deps.IntelQuerier.GetIntelSource(ctx, gen.GetIntelSourceParams{TenantID: tenantID, ID: sourceID})
			if err != nil {
				return errorToolResult("source_not_found", "source not found"), nil
			}
			if !src.Enabled {
				return errorToolResult("source_disabled", "source is disabled; enable it before crawling"), nil
			}
		}

		payload, _ := json.Marshal(map[string]string{
			"version":   "v1",
			"source_id": a.SourceID,
			"tenant_id": tenantID.String(),
		})
		if err := deps.TaskEnqueuer.Enqueue(worker.TypeIntelFetchSource, payload); err != nil {
			return errorToolResult("enqueue_failed", fmt.Sprintf("failed to enqueue crawl: %v", err)), nil
		}

		return successToolResult(map[string]any{
			"status":    "queued",
			"task":      worker.TypeIntelFetchSource,
			"source_id": a.SourceID,
		}), nil
	}
}

// ---------------------------------------------------------------------------
// record.update_now
// ---------------------------------------------------------------------------

type recordUpdateNowArgs struct {
	RecordID string `json:"record_id,omitempty"`
	SourceID string `json:"source_id,omitempty"`
}

// RecordUpdateNowHandler returns a ToolHandler that triggers the full
// intelligence fetch chain for a record or source. If record_id is given,
// the source is resolved from the record; otherwise source_id is used directly.
func RecordUpdateNowHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.TaskEnqueuer == nil {
			return errorToolResult("task_enqueuer_unavailable", "task enqueuer is not configured"), nil
		}

		var a recordUpdateNowArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.RecordID == "" && a.SourceID == "" {
			return errorToolResult("missing_record_or_source", "record_id or source_id is required"), nil
		}

		sourceID := pgtype.UUID{}
		if a.SourceID != "" {
			id, err := scanUUID(a.SourceID)
			if err != nil {
				return errorToolResult("invalid_source_id", "source_id is not a valid UUID"), nil
			}
			sourceID = id
		}

		if a.RecordID != "" {
			recordID, err := scanUUID(a.RecordID)
			if err != nil {
				return errorToolResult("invalid_record_id", "record_id is not a valid UUID"), nil
			}
			if deps.IntelQuerier == nil {
				return errorToolResult("intel_unavailable", "intel querier is not configured"), nil
			}
			record, err := deps.IntelQuerier.GetIntelRecord(ctx, gen.GetIntelRecordParams{TenantID: tenantID, ID: recordID})
			if err != nil {
				return errorToolResult("record_not_found", "intel record not found"), nil
			}
			sourceID = record.SourceID
		}

		if !sourceID.Valid {
			return errorToolResult("missing_source", "could not resolve a source for update"), nil
		}

		if deps.IntelQuerier != nil {
			src, err := deps.IntelQuerier.GetIntelSource(ctx, gen.GetIntelSourceParams{TenantID: tenantID, ID: sourceID})
			if err != nil {
				return errorToolResult("source_not_found", "source not found"), nil
			}
			if !src.Enabled {
				return errorToolResult("source_disabled", "source is disabled; enable it before updating"), nil
			}
		}

		payload, _ := json.Marshal(map[string]string{
			"version":   "v1",
			"source_id": sourceID.String(),
			"tenant_id": tenantID.String(),
		})
		if err := deps.TaskEnqueuer.Enqueue(worker.TypeIntelFetchSource, payload); err != nil {
			return errorToolResult("enqueue_failed", fmt.Sprintf("failed to enqueue update: %v", err)), nil
		}

		return successToolResult(map[string]any{
			"status":    "queued",
			"task":      worker.TypeIntelFetchSource,
			"source_id": sourceID.String(),
		}), nil
	}
}

// ---------------------------------------------------------------------------
// content.regenerate_from_source
// ---------------------------------------------------------------------------

type contentRegenerateFromSourceArgs struct {
	ContentID     string `json:"content_id"`
	IntelRecordID string `json:"intel_record_id"`
}

// ContentRegenerateFromSourceHandler returns a ToolHandler that enqueues the
// content:regenerate_from_source worker to rewrite an existing contents row
// from a linked intelligence record.
func ContentRegenerateFromSourceHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.TaskEnqueuer == nil {
			return errorToolResult("task_enqueuer_unavailable", "task enqueuer is not configured"), nil
		}

		var a contentRegenerateFromSourceArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.ContentID == "" {
			return errorToolResult("missing_content_id", "content_id is required"), nil
		}
		if a.IntelRecordID == "" {
			return errorToolResult("missing_intel_record_id", "intel_record_id is required"), nil
		}

		if _, err := scanUUID(a.ContentID); err != nil {
			return errorToolResult("invalid_content_id", "content_id is not a valid UUID"), nil
		}
		intelRecordID, err := scanUUID(a.IntelRecordID)
		if err != nil {
			return errorToolResult("invalid_intel_record_id", "intel_record_id is not a valid UUID"), nil
		}

		if deps.IntelQuerier != nil {
			if _, err := deps.IntelQuerier.GetIntelRecord(ctx, gen.GetIntelRecordParams{TenantID: tenantID, ID: intelRecordID}); err != nil {
				return errorToolResult("record_not_found", "intel record not found"), nil
			}
		}

		payload, _ := json.Marshal(map[string]string{
			"content_id":      a.ContentID,
			"intel_record_id": a.IntelRecordID,
			"tenant_id":       tenantID.String(),
		})
		if err := deps.TaskEnqueuer.Enqueue(worker.TypeContentRegenerateFromSource, payload); err != nil {
			return errorToolResult("enqueue_failed", fmt.Sprintf("failed to enqueue regeneration: %v", err)), nil
		}

		return successToolResult(map[string]any{
			"status":          "queued",
			"task":            worker.TypeContentRegenerateFromSource,
			"content_id":      a.ContentID,
			"intel_record_id": a.IntelRecordID,
		}), nil
	}
}
