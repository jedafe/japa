// Package ai — tools_intel_read.go implements read-only intelligence tools
// that close the loop for the enqueue-only intel tools: after
// source.crawl_now / record.update_now return "queued", the agent (and MCP
// callers on the same executor) can inspect the resulting sources and
// extracted records instead of firing blind.
//
// Tools implemented here:
//
//   - intel.source.get   source registry row: status/enabled/trust/crawl info
//   - intel.record.get   full intel_records row incl. extracted + evidence JSONB
//   - intel.record.list  lightweight record browser (source/status/entity_type
//     filters; heavy payloads omitted — use record.get)
//
// All are read-only platform data (no user content), so no privacy targets
// are registered. The catalog RBAC gate enforces intel.sources.view /
// intel.review.view per the ai_tools rows (migration 000842).
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Tool Catalog)
// Ref: docs/superpowers/specs/2026-08-19-on-demand-intel-to-content.md
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// intelRecordListCap bounds a single intel.record.list page — the extraction
// catalog is large and agent context windows are not.
const intelRecordListCap = 25

func tsOrNil(t pgtype.Timestamptz) any {
	if !t.Valid {
		return nil
	}

	return t.Time.Format(time.RFC3339)
}

// rawOrNil wraps a JSONB column as json.RawMessage so the payload embeds
// verbatim instead of base64-encoding under encoding/json.
func rawOrNil(b []byte) any {
	if len(b) == 0 {
		return nil
	}

	return json.RawMessage(b)
}

// ---------------------------------------------------------------------------
// intel.source.get
// ---------------------------------------------------------------------------

type intelSourceGetArgs struct {
	SourceID string `json:"source_id"`
}

// IntelSourceGetHandler returns a ToolHandler that reads a single intel
// source registry row — status, enabled flag, trust, crawl health.
func IntelSourceGetHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.IntelQuerier == nil {
			return errorToolResult("intel_unavailable", "intel querier is not configured"), nil
		}

		var a intelSourceGetArgs
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

		src, err := deps.IntelQuerier.GetIntelSource(ctx, sourceID)
		if err != nil {
			return errorToolResult("source_not_found", "intel source not found"), nil
		}

		return successToolResult(map[string]any{
			"id":                   uuidString(src.ID),
			"name":                 src.Name,
			"domain":               src.Domain,
			"source_url":           src.SourceUrl,
			"description":          src.Description,
			"category":             src.Category,
			"categories":           src.Categories,
			"source_type":          src.SourceType,
			"source_kind":          src.SourceKind,
			"trust_level":          src.TrustLevel,
			"crawl_method":         src.CrawlMethod,
			"crawl_frequency":      src.CrawlFrequency,
			"enabled":              src.Enabled,
			"status":               src.Status,
			"last_crawled_at":      tsOrNil(src.LastCrawledAt),
			"last_success_at":      tsOrNil(src.LastSuccessAt),
			"last_error_at":        tsOrNil(src.LastErrorAt),
			"last_error_message":   src.LastErrorMessage,
			"consecutive_failures": src.ConsecutiveFailures,
			"proposed_at":          tsOrNil(src.ProposedAt),
			"approved_at":          tsOrNil(src.ApprovedAt),
		}), nil
	}
}

// ---------------------------------------------------------------------------
// intel.record.get
// ---------------------------------------------------------------------------

type intelRecordGetArgs struct {
	RecordID string `json:"record_id"`
}

// IntelRecordGetHandler returns a ToolHandler that reads a single intel
// record including the full extracted payload, confidence map, and evidence —
// the "what did extraction produce" read.
func IntelRecordGetHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.IntelQuerier == nil {
			return errorToolResult("intel_unavailable", "intel querier is not configured"), nil
		}

		var a intelRecordGetArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if a.RecordID == "" {
			return errorToolResult("missing_record_id", "record_id is required"), nil
		}

		recordID, err := scanUUID(a.RecordID)
		if err != nil {
			return errorToolResult("invalid_record_id", "record_id is not a valid UUID"), nil
		}

		rec, err := deps.IntelQuerier.GetIntelRecord(ctx, recordID)
		if err != nil {
			return errorToolResult("record_not_found", "intel record not found"), nil
		}

		return successToolResult(map[string]any{
			"id":                 uuidString(rec.ID),
			"source_id":          uuidString(rec.SourceID),
			"entity_type":        rec.EntityType,
			"external_key":       rec.ExternalKey,
			"status":             rec.Status,
			"confidence_overall": rec.ConfidenceOverall,
			"freshness_score":    rec.FreshnessScore,
			"extracted":          rawOrNil(rec.Extracted),
			"confidence":         rawOrNil(rec.Confidence),
			"evidence":           rawOrNil(rec.Evidence),
			"extracted_at":       tsOrNil(rec.ExtractedAt),
			"last_verified_at":   tsOrNil(rec.LastVerifiedAt),
			"promoted_record_id": uuidString(rec.PromotedRecordID),
		}), nil
	}
}

// ---------------------------------------------------------------------------
// intel.record.list
// ---------------------------------------------------------------------------

type intelRecordListArgs struct {
	SourceID   string `json:"source_id,omitempty"`
	Status     string `json:"status,omitempty"`
	EntityType string `json:"entity_type,omitempty"`
	Limit      int32  `json:"limit,omitempty"`
	Offset     int32  `json:"offset,omitempty"`
}

// IntelRecordListHandler returns a ToolHandler that browses intel records
// with optional source/status/entity_type filters. Payloads are omitted —
// call intel.record.get for the full extraction.
func IntelRecordListHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.IntelQuerier == nil {
			return errorToolResult("intel_unavailable", "intel querier is not configured"), nil
		}

		var a intelRecordListArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		params := gen.ListIntelRecordsForAgentParams{
			RowLimit:  intelRecordListCap,
			RowOffset: a.Offset,
		}
		if a.Limit > 0 && a.Limit < intelRecordListCap {
			params.RowLimit = a.Limit
		}

		if params.RowOffset < 0 {
			params.RowOffset = 0
		}

		if a.SourceID != "" {
			sourceID, err := scanUUID(a.SourceID)
			if err != nil {
				return errorToolResult("invalid_source_id", "source_id is not a valid UUID"), nil
			}

			params.SourceID = sourceID
		}

		if a.Status != "" {
			params.Status = &a.Status
		}

		if a.EntityType != "" {
			params.EntityType = &a.EntityType
		}

		rows, err := deps.IntelQuerier.ListIntelRecordsForAgent(ctx, params)
		if err != nil {
			return errorToolResult("query_failed", fmt.Sprintf("record list failed: %v", err)), nil
		}

		records := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			records = append(records, map[string]any{
				"id":                 uuidString(r.ID),
				"source_id":          uuidString(r.SourceID),
				"entity_type":        r.EntityType,
				"external_key":       r.ExternalKey,
				"status":             r.Status,
				"confidence_overall": r.ConfidenceOverall,
				"freshness_score":    r.FreshnessScore,
				"extracted_at":       tsOrNil(r.ExtractedAt),
				"last_verified_at":   tsOrNil(r.LastVerifiedAt),
				"promoted_record_id": uuidString(r.PromotedRecordID),
			})
		}

		return successToolResult(map[string]any{
			"records": records,
			"count":   len(records),
			"limit":   params.RowLimit,
			"offset":  params.RowOffset,
		}), nil
	}
}
