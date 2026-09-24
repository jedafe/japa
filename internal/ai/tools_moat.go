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

// ---------------------------------------------------------------------------
// Phase 2 — feeds, scam alerts, semantic search
// Ref: docs/superpowers/specs/2026-09-24-agent-data-api-phase2-design.md
// ---------------------------------------------------------------------------

type feedsExploreArgs struct {
	Name      string `json:"name,omitempty"`
	SpaceSlug string `json:"space_slug,omitempty"`
	Tag       string `json:"tag,omitempty"`
	Sort      string `json:"sort,omitempty"`
	Limit     int32  `json:"limit,omitempty"`
	Offset    int32  `json:"offset,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
}

// FeedsExploreHandler implements the feeds.explore agent tool — the anonymous
// explore feeds plus the tag and space variants, same read path as
// GET /public/feeds/*. Variant selection: tag → tag feed (cursor-paginated),
// space_slug → space feed (resolved as the calling user so member-only items
// match the web feed), otherwise the named explore feed.
func FeedsExploreHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.AgentData == nil {
			return errorToolResult("feeds_explore_unavailable", "feeds are not configured"), nil
		}
		var a feedsExploreArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.Limit <= 0 || a.Limit > 50 {
			a.Limit = 20
		}
		if a.Offset < 0 {
			a.Offset = 0
		}
		tag := strings.TrimSpace(a.Tag)
		slug := strings.TrimSpace(a.SpaceSlug)
		sort := strings.TrimSpace(a.Sort)
		switch {
		case tag != "":
			feed, err := deps.AgentData.TagFeed(ctx, tag, strings.TrimSpace(a.Cursor), a.Limit)
			if err != nil {
				return errorToolResult("feeds_explore_failed", err.Error()), nil
			}
			return successToolResult(map[string]any{"feed": feed}), nil
		case slug != "":
			feed, err := deps.AgentData.SpaceFeed(ctx, userID, slug, sort, a.Limit, a.Offset)
			if err != nil {
				return errorToolResult("feeds_explore_failed", err.Error()), nil
			}
			return successToolResult(map[string]any{"feed": feed}), nil
		default:
			name := strings.TrimSpace(a.Name)
			if name == "" {
				name = "popular"
			}
			feed, err := deps.AgentData.ExploreFeed(ctx, name, sort, a.Limit, a.Offset)
			if err != nil {
				return errorToolResult("feeds_explore_failed", err.Error()), nil
			}
			return successToolResult(map[string]any{"feed": feed}), nil
		}
	}
}

type feedsForYouArgs struct {
	Sort   string `json:"sort,omitempty"`
	Limit  int32  `json:"limit,omitempty"`
	Offset int32  `json:"offset,omitempty"`
}

// FeedsForYouHandler implements the feeds.for_you agent tool — the calling
// user's ranked for-you feed, same read path as GET /public/me/feed. The
// caller identity is the tool-call userID (session user or API-key owner).
func FeedsForYouHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.AgentData == nil {
			return errorToolResult("feeds_for_you_unavailable", "feeds are not configured"), nil
		}
		if !userID.Valid {
			return errorToolResult("unauthenticated", "feeds.for_you requires an authenticated caller"), nil
		}
		var a feedsForYouArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.Limit <= 0 || a.Limit > 50 {
			a.Limit = 20
		}
		if a.Offset < 0 {
			a.Offset = 0
		}
		feed, err := deps.AgentData.MyFeed(ctx, userID, strings.TrimSpace(a.Sort), a.Limit, a.Offset)
		if err != nil {
			return errorToolResult("feeds_for_you_failed", err.Error()), nil
		}
		return successToolResult(map[string]any{"feed": feed}), nil
	}
}

type scamListArgs struct {
	DestSpaceID string `json:"destination_space_id,omitempty"`
	ScamType    string `json:"scam_type,omitempty"`
	Limit       int32  `json:"limit,omitempty"`
	Offset      int32  `json:"offset,omitempty"`
}

// ScamListHandler implements the scam.list agent tool — confirmed scam
// reports, same read path as GET /public/scam-alerts.
func ScamListHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.AgentData == nil {
			return errorToolResult("scam_list_unavailable", "scam alerts are not configured"), nil
		}
		var a scamListArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.Limit <= 0 || a.Limit > 50 {
			a.Limit = 20
		}
		if a.Offset < 0 {
			a.Offset = 0
		}
		var dest pgtype.UUID
		if a.DestSpaceID != "" {
			if err := dest.Scan(strings.TrimSpace(a.DestSpaceID)); err != nil {
				return errorToolResult("invalid_arguments", "invalid destination_space_id"), nil
			}
		}
		alerts, total, err := deps.AgentData.ListScamAlerts(ctx, dest, strings.TrimSpace(a.ScamType), a.Limit, a.Offset)
		if err != nil {
			return errorToolResult("scam_list_failed", err.Error()), nil
		}
		return successToolResult(map[string]any{"alerts": alerts, "total": total}), nil
	}
}

type scamGetArgs struct {
	ID string `json:"id"`
}

// ScamGetHandler implements the scam.get agent tool — a single confirmed
// scam report by ID, same read path as GET /public/scam-alerts/{id}.
func ScamGetHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.AgentData == nil {
			return errorToolResult("scam_get_unavailable", "scam alerts are not configured"), nil
		}
		var a scamGetArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		id, err := scanUUID(strings.TrimSpace(a.ID))
		if err != nil {
			return errorToolResult("invalid_alert_id", "id is required and must be a valid UUID"), nil
		}
		alert, err := deps.AgentData.GetScamAlert(ctx, id)
		if err != nil {
			return errorToolResult("scam_get_failed", err.Error()), nil
		}
		return successToolResult(map[string]any{"alert": alert}), nil
	}
}

type semanticSearchArgs struct {
	Query string `json:"query"`
	Lang  string `json:"lang,omitempty"`
	Space string `json:"space,omitempty"`
	Limit int32  `json:"limit,omitempty"`
}

// SemanticSearchHandler implements the search.semantic agent tool — pgvector
// retrieval over indexed entities, same read path (and Valkey result cache)
// as GET /public/search/semantic. Premium: the MCP gate enforces the pro
// tier, mirroring RequireTier on the REST route.
func SemanticSearchHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.AgentData == nil {
			return errorToolResult("search_semantic_unavailable", "semantic search is not configured"), nil
		}
		var a semanticSearchArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		query := strings.TrimSpace(a.Query)
		if query == "" {
			return errorToolResult("missing_query", "query is required"), nil
		}
		result, err := deps.AgentData.SemanticSearch(ctx, query, strings.TrimSpace(a.Lang), strings.TrimSpace(a.Space), a.Limit)
		if err != nil {
			return errorToolResult("search_semantic_failed", err.Error()), nil
		}
		return successToolResult(map[string]any{"result": result}), nil
	}
}
