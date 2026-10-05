// Package ai — tools_wiki.go implements the Destination Wiki agent tools
// (Section 80, Section 103). Wiki body content is revision-gated: the
// tools here create *pending* revisions through the same Service methods
// the REST routes use, so agent-produced prose still passes moderator
// review. Page flag/provisioning ops (wiki_page.update/.delete and the
// content-less create) stay on the generic catalog writer.
//
// Ref: japaconnect-engineering/sections/80-destination-wiki.md
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// wiki_page.create — dual mode: with `content` the page is created with an
// initial pending revision (CreatePageWithRevision); without it, a
// provisioning shell page is created (Store.CreatePage). Accepts the
// standard catalog-op {payload: {...}} argument shape.
// ---------------------------------------------------------------------------

// WikiPageCreateHandler returns a ToolHandler for wiki_page.create. It
// replaces the generic writer registration (registered after the
// WritableToolOps loop) so content-bearing creates flow through the §80
// revision queue. When deps.Wiki is unwired the handler falls back to
// the generic registry writer for content-less shell creates — a payload
// carrying content still reports wiki_unavailable rather than silently
// dropping the body.
func WikiPageCreateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		payload, err := payloadFromArgs(args)
		if err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if deps == nil || deps.Wiki == nil {
			return wikiCreateFallback(ctx, deps, userID, payload)
		}

		slug, _ := payload["slug"].(string)
		if slug == "" {
			return errorToolResult("missing_slug", "slug is required"), nil
		}

		title, _ := payload["title"].(string)
		if title == "" {
			return errorToolResult("missing_title", "title is required"), nil
		}

		category, _ := payload["category"].(string)
		if category == "" {
			category = "general"
		}

		content, err := rawContent(payload["content"])
		if err != nil {
			return errorToolResult("invalid_content", fmt.Sprintf("invalid content: %v", err)), nil
		}

		if len(content) > 0 {
			return wikiCreateWithRevision(ctx, deps, userID, payload, slug, title, category, content)
		}

		// Content-less payload → provisioning shell create (unchanged
		// behaviour for callers that manage page bodies separately).
		spaceID, err := wikiSpaceID(ctx, deps.Wiki, payload)
		if err != nil {
			return errorToolResult("invalid_space", err.Error()), nil
		}

		pageID, err := deps.Wiki.CreatePageShell(ctx, spaceID, userID, slug, title, category)
		if err != nil {
			return errorToolResult("wiki_create_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"page_id": pageID,
			"slug":    slug,
			"status":  "created",
		}), nil
	}
}

// wikiCreateFallback handles wiki_page.create when the dedicated wiki
// service is unwired: content-less payloads degrade to the generic
// registry writer (shell create); a content-bearing payload is refused
// instead of silently dropping the body into a shell page.
func wikiCreateFallback(ctx context.Context, deps *ToolDeps, userID pgtype.UUID, payload map[string]any) (*ToolResult, error) {
	if content, _ := rawContent(payload["content"]); len(content) > 0 {
		return errorToolResult("wiki_unavailable", "wiki service is not configured; content-bearing creates need the §80 revision path"), nil
	}
	if deps == nil || deps.ToolsRegistry == nil {
		return errorToolResult("wiki_unavailable", "wiki service is not configured"), nil
	}

	record, err := deps.ToolsRegistry.CreateTool(ctx, userID, "wiki_page", payload)
	if err != nil {
		return errorToolResult("wiki_create_failed", err.Error()), nil
	}

	return successToolResult(map[string]any{
		"tool_type": "wiki_page",
		"record":    record,
	}), nil
}

// wikiCreateWithRevision handles the content-bearing branch of
// wiki_page.create — page + initial pending revision in one transaction.
func wikiCreateWithRevision(ctx context.Context, deps *ToolDeps, userID pgtype.UUID, payload map[string]any, slug, title, category string, content json.RawMessage) (*ToolResult, error) {
	spaceSlug, _ := payload["space_slug"].(string)
	if spaceSlug == "" {
		id, err := wikiSpaceID(ctx, deps.Wiki, payload)
		if err != nil {
			return errorToolResult("invalid_space", err.Error()), nil
		}
		spaceSlug, err = deps.Wiki.SpaceSlugByID(ctx, id)
		if err != nil {
			return errorToolResult("invalid_space", fmt.Sprintf("could not resolve space: %v", err)), nil
		}
	}

	var position *int32
	if v, ok := payload["position"]; ok && v != nil {
		if f, ok := v.(float64); ok {
			p := int32(f) //nolint:gosec // position is a small ordinal
			position = &p
		}
	}

	summary, _ := payload["summary"].(string)

	pageID, revID, pageSlug, err := deps.Wiki.CreatePageWithRevision(ctx, userID, WikiCreatePageInput{
		SpaceSlug: spaceSlug,
		Slug:      slug,
		Title:     title,
		Category:  category,
		Content:   content,
		Summary:   summary,
		Position:  position,
	})
	if err != nil {
		return errorToolResult("wiki_create_failed", err.Error()), nil
	}

	return successToolResult(map[string]any{
		"page_id":     pageID,
		"revision_id": revID,
		"slug":        pageSlug,
		"status":      "pending",
	}), nil
}

// ---------------------------------------------------------------------------
// wiki_page.submit_revision — body edit on an existing page, stored as a
// pending revision (§80). Mirrors the REST gates: community_edits and the
// min_reputation_to_edit threshold.
// ---------------------------------------------------------------------------

type wikiSubmitRevisionArgs struct {
	SpaceSlug string          `json:"space_slug"`
	PageSlug  string          `json:"page_slug"`
	Content   json.RawMessage `json:"content"`
	// Ops carries §80 community field ops (structured edits). When present
	// the submission stores a single fields/mixed revision — prose and ops
	// approve atomically, matching the REST route.
	Ops     json.RawMessage `json:"ops"`
	Summary string          `json:"summary"`
}

// WikiPageSubmitRevisionHandler returns a ToolHandler for
// wiki_page.submit_revision — the agent path for wiki content and
// structured-field updates.
func WikiPageSubmitRevisionHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Wiki == nil {
			return errorToolResult("wiki_unavailable", "wiki service is not configured"), nil
		}

		var a wikiSubmitRevisionArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if a.SpaceSlug == "" || a.PageSlug == "" {
			return errorToolResult("missing_slug", "space_slug and page_slug are required"), nil
		}

		hasOps := len(a.Ops) > 0 && string(a.Ops) != "null" && string(a.Ops) != "[]"
		if len(a.Content) == 0 && !hasOps {
			return errorToolResult("missing_content", "content or field ops are required"), nil
		}

		// §80 gates, mirroring HandleSubmitRevision: when community edits
		// are disabled only wiki.review/wiki.manage holders may submit.
		isMod := deps.Permission != nil &&
			(deps.Permission.HasPermission(ctx, userID, "wiki.review") ||
				deps.Permission.HasPermission(ctx, userID, "wiki.manage"))
		if !deps.Wiki.CommunityEditsAllowed(ctx) && !isMod {
			return errorToolResult("community_edits_disabled", "community edits are currently disabled for this wiki"), nil
		}

		// Reputation gate (wiki.min_reputation_to_edit). Fail closed when
		// the points reader is unavailable — same convention as the REST
		// handler's BUG-004 fix. On the page route the gate applies to
		// prose and ops submissions alike.
		if threshold := deps.Wiki.MinReputationToEdit(ctx); threshold > 0 {
			if deps.Points == nil {
				return errorToolResult("reputation_check_unavailable", "reputation verification is currently unavailable"), nil
			}
			points, err := deps.Points.GetPointsBalance(ctx, userID)
			if err != nil {
				return errorToolResult("reputation_check_failed", "failed to verify reputation"), nil
			}
			if points < int64(threshold) {
				return errorToolResult("insufficient_reputation", "you need more reputation to edit this wiki page"), nil
			}
		}

		// Field-ops path (§80 structured edits): gated by the
		// wiki.structured_edits feature flag; wiki.fields.manage holders
		// auto-apply their ops instead of queueing.
		if hasOps {
			if !deps.Wiki.StructuredEditsEnabled(ctx) {
				return errorToolResult("feature_disabled", "structured field edits are not available"), nil
			}

			canManage := deps.Permission != nil &&
				deps.Permission.HasPermission(ctx, userID, "wiki.fields.manage")

			revID, autoApproved, err := deps.Wiki.SubmitFieldOps(ctx, userID, a.SpaceSlug, a.PageSlug, a.Content, a.Ops, a.Summary, canManage)
			if err != nil {
				return errorToolResult("wiki_submit_failed", err.Error()), nil
			}

			status := "pending"
			if autoApproved {
				status = "approved"
			}

			return successToolResult(map[string]any{
				"revision_id":   revID,
				"status":        status,
				"auto_approved": autoApproved,
			}), nil
		}

		revID, err := deps.Wiki.SubmitRevision(ctx, userID, a.SpaceSlug, a.PageSlug, a.Content, a.Summary)
		if err != nil {
			return errorToolResult("wiki_submit_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"revision_id": revID,
			"status":      "pending",
		}), nil
	}
}

// ---------------------------------------------------------------------------
// wiki_page.review_revision — approve or reject a pending revision (the
// agent equivalent of POST /admin/wiki/revisions/:id/approve|reject).
// ---------------------------------------------------------------------------

type wikiReviewRevisionArgs struct {
	RevisionID string `json:"revision_id"`
	Action     string `json:"action"` // "approve" | "reject"
	Notes      string `json:"notes"`
}

// WikiPageReviewRevisionHandler returns a ToolHandler for
// wiki_page.review_revision. Required permission (seeded): wiki.review.
func WikiPageReviewRevisionHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Wiki == nil {
			return errorToolResult("wiki_unavailable", "wiki service is not configured"), nil
		}

		var a wikiReviewRevisionArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		revID, err := scanUUID(a.RevisionID)
		if err != nil {
			return errorToolResult("invalid_revision_id", "revision_id must be a UUID"), nil
		}

		switch a.Action {
		case "approve":
			if err := deps.Wiki.ApproveRevision(ctx, revID, userID, a.Notes); err != nil {
				return errorToolResult("wiki_approve_failed", err.Error()), nil
			}
		case "reject":
			if err := deps.Wiki.RejectRevision(ctx, revID, userID, a.Notes); err != nil {
				return errorToolResult("wiki_reject_failed", err.Error()), nil
			}
		default:
			return errorToolResult("invalid_action", "action must be \"approve\" or \"reject\""), nil
		}

		return successToolResult(map[string]any{
			"revision_id": a.RevisionID,
			"action":      a.Action,
			"status":      "done",
		}), nil
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// rawContent converts a payload `content` value to json.RawMessage. The
// value may be a TipTap doc object (map) or a plain string (markdown/HTML —
// the service normalizes either form per BUG-020).
func rawContent(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if string(raw) == `""` || string(raw) == "null" {
		return nil, nil
	}
	return raw, nil
}

// wikiSpaceID resolves the target space UUID from payload keys
// space_id (UUID) or space_slug.
func wikiSpaceID(ctx context.Context, wiki WikiAgentService, payload map[string]any) (pgtype.UUID, error) {
	if s, _ := payload["space_id"].(string); s != "" {
		return scanUUID(s)
	}
	if slug, _ := payload["space_slug"].(string); slug != "" {
		return wiki.SpaceIDBySlug(ctx, slug)
	}
	return pgtype.UUID{}, fmt.Errorf("space_id or space_slug is required")
}
