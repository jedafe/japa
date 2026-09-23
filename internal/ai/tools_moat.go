// Package ai — tools_moat.go implements the agent data tools backed by the
// §47 public API moat facade: visa.search, visa.get, visa.compare,
// university.get, and destination.brief.
//
// The ai package cannot import internal/publicapi or the moat domain
// packages (visaroutes and languagetesthub already import internal/ai for
// their SetAIDispatcher hooks — an ai → publicapi → visaroutes edge would
// be a compile-breaking import cycle). The handlers therefore call the
// ai-local AgentDataReader interface whose methods return opaque any
// payloads; the wiring layer (agentDataAdapter in package server)
// delegates to *publicapi.Service so REST and MCP share the same read path
// and can never drift.
//
// Ref: docs/superpowers/specs/2026-09-23-agent-data-api-design.md
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

type visaSearchArgs struct {
	DestSpaceID string `json:"destination_space_id,omitempty"`
	Profession  string `json:"profession,omitempty"`
	Education   string `json:"education,omitempty"`
	Years       int32  `json:"experience_years,omitempty"`
	Limit       int32  `json:"limit,omitempty"`
	Page        int32  `json:"page,omitempty"`
}

// VisaSearchHandler implements the visa.search agent tool. Results are the
// publicapi facade's RouteDTO list embedded verbatim — same read path as
// GET /public/visa-routes so REST and MCP can never drift.
func VisaSearchHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.AgentData == nil {
			return errorToolResult("visa_search_unavailable", "visa search is not configured"), nil
		}
		var a visaSearchArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.Limit <= 0 || a.Limit > 50 {
			a.Limit = 20
		}
		if a.Page <= 0 {
			a.Page = 1
		}
		var dest pgtype.UUID
		if a.DestSpaceID != "" {
			if err := dest.Scan(strings.TrimSpace(a.DestSpaceID)); err != nil {
				return errorToolResult("invalid_arguments", "invalid destination_space_id"), nil
			}
		}
		routes, total, err := deps.AgentData.ListVisaRoutes(ctx, dest, a.Profession, a.Education, a.Years, a.Page, a.Limit)
		if err != nil {
			return errorToolResult("visa_search_failed", err.Error()), nil
		}
		return successToolResult(map[string]any{"routes": routes, "total": total}), nil
	}
}

type visaGetArgs struct {
	RouteID string `json:"route_id"`
}

// VisaGetHandler implements the visa.get agent tool — a single visa route
// record by ID, same read path as GET /public/visa-routes/{id}.
func VisaGetHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.AgentData == nil {
			return errorToolResult("visa_get_unavailable", "visa route lookup is not configured"), nil
		}
		var a visaGetArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		id, err := scanUUID(strings.TrimSpace(a.RouteID))
		if err != nil {
			return errorToolResult("invalid_route_id", "route_id is required and must be a valid UUID"), nil
		}
		route, err := deps.AgentData.GetVisaRoute(ctx, id)
		if err != nil {
			return errorToolResult("visa_get_failed", err.Error()), nil
		}
		return successToolResult(map[string]any{"route": route}), nil
	}
}

type visaCompareArgs struct {
	RouteIDs []string `json:"route_ids"`
}

// VisaCompareHandler implements the visa.compare agent tool — up to 5 visa
// routes side by side, same read path as GET /public/visa-routes/compare.
func VisaCompareHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.AgentData == nil {
			return errorToolResult("visa_compare_unavailable", "visa route comparison is not configured"), nil
		}
		var a visaCompareArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if len(a.RouteIDs) == 0 {
			return errorToolResult("missing_route_ids", "route_ids is required"), nil
		}
		if len(a.RouteIDs) > 5 {
			return errorToolResult("too_many_route_ids", "route_ids accepts at most 5 route IDs"), nil
		}
		ids := make([]pgtype.UUID, 0, len(a.RouteIDs))
		for _, raw := range a.RouteIDs {
			id, err := scanUUID(strings.TrimSpace(raw))
			if err != nil {
				return errorToolResult("invalid_route_id", fmt.Sprintf("route_id %q is not a valid UUID", raw)), nil
			}
			ids = append(ids, id)
		}
		routes, err := deps.AgentData.CompareVisaRoutes(ctx, ids)
		if err != nil {
			return errorToolResult("visa_compare_failed", err.Error()), nil
		}
		return successToolResult(map[string]any{"routes": routes}), nil
	}
}

type universityGetArgs struct {
	Slug string `json:"slug"`
}

// UniversityGetHandler implements the university.get agent tool — a single
// university space by slug, same read path as GET /public/universities/{slug}.
func UniversityGetHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.AgentData == nil {
			return errorToolResult("university_get_unavailable", "university lookup is not configured"), nil
		}
		var a universityGetArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		slug := strings.TrimSpace(a.Slug)
		if slug == "" {
			return errorToolResult("missing_slug", "slug is required"), nil
		}
		university, err := deps.AgentData.GetUniversity(ctx, slug)
		if err != nil {
			return errorToolResult("university_get_failed", err.Error()), nil
		}
		return successToolResult(map[string]any{"university": university}), nil
	}
}

type destinationBriefArgs struct {
	Slug    string   `json:"slug"`
	Include []string `json:"include,omitempty"`
}

// DestinationBriefHandler implements the destination.brief agent tool —
// the aggregate destination brief, same read path (and response cache) as
// GET /public/destinations/{slug}/brief.
func DestinationBriefHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.AgentData == nil {
			return errorToolResult("destination_brief_unavailable", "destination brief is not configured"), nil
		}
		var a destinationBriefArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		slug := strings.TrimSpace(a.Slug)
		if slug == "" {
			return errorToolResult("missing_slug", "slug is required"), nil
		}
		brief, err := deps.AgentData.GetDestinationBrief(ctx, slug, a.Include)
		if err != nil {
			return errorToolResult("destination_brief_failed", err.Error()), nil
		}
		return successToolResult(map[string]any{"brief": brief}), nil
	}
}
