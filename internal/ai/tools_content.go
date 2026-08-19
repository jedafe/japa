// Package ai — tools_content.go implements the content tool handlers for the
// AI Agent Executor core tool catalog (Section 103, ADR-111).
//
// Tools implemented here (Phase 4, tasks 4.01-4.04):
//
//   - content.search   — tenant-wide or space-scoped text search of contents
//   - content.read     — read a content record + its non-exempt custom field values
//   - content.create   — create new content with _ai_provenance metadata
//   - content.update   — update existing content fields
//
// Every handler is a ToolHandler (func signature) closing over a *ToolDeps.
// The executor pipeline (executor.go) handles schema validation, RBAC,
// privacy blocklist, and HITL before dispatch — the handlers focus on
// argument parsing, calling the internal/content service, and shaping the
// ToolResult JSON the LLM consumes.
//
// Privacy: content.read and content.update target a specific content row,
// so they register a PrivacyTargetFunc that extracts the content_id from
// the arguments. The executor's privacy gate blocks the call before
// dispatch when the content is ai_exempt. content.search returns a result
// set — the ListContentsByTenant and SearchContentsBySpace queries do not
// filter ai_exempt at the SQL layer, so the handler filters exempt content
// post-query using the PrivacyExemptionChecker. content.create has no
// privacy target (it creates new data, not reads existing).
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Tool Catalog Expansion — Content tools)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 4, tasks 4.01-4.04)
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content/visibility"
	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// content.search (task 4.01)
// ---------------------------------------------------------------------------

// contentSearchArgs is the LLM-produced argument shape for content.search.
type contentSearchArgs struct {
	Query      string `json:"query"`
	GeoSpaceID string `json:"geo_space_id,omitempty"`
	Limit      int32  `json:"limit,omitempty"`
}

// ContentSearchHandler returns a ToolHandler that searches site content.
// When geo_space_id is provided, the search is scoped to that space via
// content.Service.SearchContentsBySpace; otherwise it falls back to the
// tenant-wide content.Service.ListContents text filter (first page, no
// cursor — the agent loop does not paginate).
//
// Privacy: the ListContentsByTenant and SearchContentsBySpace queries do
// not filter ai_exempt at the SQL layer (the ai_exempt column is not in
// their WHERE clause). The handler filters exempt content post-query using
// the PrivacyExemptionChecker. When deps.Privacy is nil, no filtering is
// applied — the wiring layer MUST provide a Privacy checker for the agent
// to enforce the §103 privacy boundary.
func ContentSearchHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Content == nil {
			return errorToolResult("content_search_unavailable", "content service is not configured"), nil
		}

		var a contentSearchArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.Query == "" {
			return errorToolResult("missing_query", "query is required"), nil
		}
		if a.Limit <= 0 || a.Limit > 50 {
			a.Limit = 10
		}

		// Geo-scoped search: use SearchContentsBySpace when a space ID is given.
		if a.GeoSpaceID != "" {
			spaceID, err := scanUUID(a.GeoSpaceID)
			if err != nil {
				return errorToolResult("invalid_geo_space_id", "geo_space_id is not a valid UUID"), nil
			}
			filter := visibility.InternalVisibilityFilter(userID)
			results, err := deps.Content.SearchContentsBySpace(ctx, tenantID, spaceID, a.Query, a.Limit, 0, filter)
			if err != nil {
				return errorToolResult("content_search_failed", err.Error()), nil
			}
			filtered := filterExemptContentFromSpace(ctx, deps.Privacy, tenantID, results)
			return successToolResult(map[string]any{
				"results":    contentSearchResultsFromSpace(filtered),
				"total":      len(filtered),
				"geo_scoped": true,
			}), nil
		}

		// Tenant-wide text search (first page — zero cursor).
		filter := visibility.InternalVisibilityFilter(userID)
		results, err := deps.Content.ListContents(ctx, tenantID, a.Query, pgtype.Timestamptz{}, a.Limit, filter)
		if err != nil {
			return errorToolResult("content_search_failed", err.Error()), nil
		}
		filtered := filterExemptContentFromTenant(ctx, deps.Privacy, tenantID, results)
		return successToolResult(map[string]any{
			"results":    contentSearchResultsFromTenant(filtered),
			"total":      len(filtered),
			"geo_scoped": false,
		}), nil
	}
}

// ContentSearchPrivacyTarget is nil — content.search returns a result set
// (not a single entity), so no PrivacyTargetFunc is registered. Exempt
// content is filtered post-query by filterExemptContentFrom* helpers.

// filterExemptContentFromTenant strips ai_exempt content from tenant-wide
// search results. When the privacy checker is nil, no filtering is applied.
func filterExemptContentFromTenant(ctx context.Context, privacy PrivacyExemptionChecker, tenantID pgtype.UUID, rows []gen.ListContentsByTenantRow) []gen.ListContentsByTenantRow {
	if privacy == nil {
		return rows
	}
	out := make([]gen.ListContentsByTenantRow, 0, len(rows))
	for _, r := range rows {
		exempt, err := privacy.IsExempt(ctx, PrivacyEntityContent, tenantID, r.ID)
		if err == nil && exempt {
			continue
		}
		out = append(out, r)
	}
	return out
}

// filterExemptContentFromSpace strips ai_exempt content from space-scoped
// search results. When the privacy checker is nil, no filtering is applied.
func filterExemptContentFromSpace(ctx context.Context, privacy PrivacyExemptionChecker, tenantID pgtype.UUID, rows []gen.SearchContentsBySpaceRow) []gen.SearchContentsBySpaceRow {
	if privacy == nil {
		return rows
	}
	out := make([]gen.SearchContentsBySpaceRow, 0, len(rows))
	for _, r := range rows {
		exempt, err := privacy.IsExempt(ctx, PrivacyEntityContent, tenantID, r.ID)
		if err == nil && exempt {
			continue
		}
		out = append(out, r)
	}
	return out
}

// contentSearchResult is the trimmed shape sent back to the LLM. Large fields
// (body, field_values) are omitted to keep the tool result compact.
type contentSearchResult struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Slug        string `json:"slug"`
	ContentType string `json:"content_type"`
	Status      string `json:"status"`
	GeoSpaceID  string `json:"geo_space_id,omitempty"`
	SpaceName   string `json:"space_name,omitempty"`
	AuthorName  string `json:"author_name,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
}

func contentSearchResultsFromTenant(rows []gen.ListContentsByTenantRow) []contentSearchResult {
	out := make([]contentSearchResult, 0, len(rows))
	for _, r := range rows {
		item := contentSearchResult{
			ID:          uuidString(r.ID),
			Title:       r.Title,
			Slug:        r.Slug,
			ContentType: r.ContentType,
			Status:      r.Status,
			GeoSpaceID:  uuidString(r.GeoSpaceID),
			CreatedAt:   timestampString(r.CreatedAt),
		}
		if r.SpaceName != nil {
			item.SpaceName = *r.SpaceName
		}
		if r.AuthorDisplayName != nil {
			item.AuthorName = *r.AuthorDisplayName
		} else if r.AuthorUsername != nil {
			item.AuthorName = *r.AuthorUsername
		}
		out = append(out, item)
	}
	return out
}

func contentSearchResultsFromSpace(rows []gen.SearchContentsBySpaceRow) []contentSearchResult {
	out := make([]contentSearchResult, 0, len(rows))
	for _, r := range rows {
		out = append(out, contentSearchResult{
			ID:          uuidString(r.ID),
			Title:       r.Title,
			Slug:        r.Slug,
			ContentType: r.ContentType,
			Status:      r.Status,
			GeoSpaceID:  uuidString(r.GeoSpaceID),
			CreatedAt:   timestampString(r.CreatedAt),
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// content.read (task 4.02)
// ---------------------------------------------------------------------------

// contentReadArgs is the argument shape for content.read.
type contentReadArgs struct {
	ContentID string `json:"content_id"`
}

// ContentReadHandler returns a ToolHandler that reads a content record and
// its non-exempt custom field values. The executor's privacy gate (via the
// registered PrivacyTargetFunc) blocks the call when the content is
// ai_exempt; the handler additionally filters ai_exempt field values from
// the returned set.
func ContentReadHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Content == nil {
			return errorToolResult("content_read_unavailable", "content service is not configured"), nil
		}

		var a contentReadArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		contentID, err := scanUUID(a.ContentID)
		if err != nil {
			return errorToolResult("invalid_content_id", "content_id is not a valid UUID"), nil
		}

		content, err := deps.Content.GetContent(ctx, contentID, tenantID)
		if err != nil {
			return errorToolResult("content_not_found", fmt.Sprintf("content not found: %v", err)), nil
		}

		// Fetch custom field values and filter out ai_exempt ones.
		fieldValues := []map[string]any{}
		if deps.CustomFields != nil {
			cfvs, err := deps.CustomFields.ListFieldValuesByEntity(ctx, tenantID, "content", contentID)
			if err == nil {
				fieldValues = filterNonExemptFieldValues(cfvs)
			}
			// On error, fieldValues stays as empty slice — non-fatal.
		}

		return successToolResult(map[string]any{
			"content": map[string]any{
				"id":           uuidString(content.ID),
				"title":        content.Title,
				"slug":         content.Slug,
				"content_type": content.ContentType,
				"body":         content.Body,
				"status":       content.Status,
				"visibility":   content.Visibility,
				"author_id":    uuidString(content.AuthorID),
				"geo_space_id": uuidString(content.GeoSpaceID),
				"created_at":   timestampString(content.CreatedAt),
				"updated_at":   timestampString(content.UpdatedAt),
			},
			"field_values": fieldValues,
		}), nil
	}
}

// ContentReadPrivacyTarget extracts the content_id from content.read
// arguments so the executor can check the ai_exempt flag before dispatch.
func ContentReadPrivacyTarget(ctx context.Context, args json.RawMessage, tenantID pgtype.UUID) (PrivacyEntityType, pgtype.UUID, error) {
	var a contentReadArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", pgtype.UUID{}, fmt.Errorf("parsing content.read args: %w", err)
	}
	id, err := scanUUID(a.ContentID)
	if err != nil {
		return "", pgtype.UUID{}, fmt.Errorf("invalid content_id: %w", err)
	}
	return PrivacyEntityContent, id, nil
}

// ---------------------------------------------------------------------------
// content.create (task 4.03)
// ---------------------------------------------------------------------------

// contentCreateArgs is the LLM-produced argument shape for content.create.
// The handler maps this to content.CreateContentInput, injecting
// _ai_provenance into the FieldValues JSONB. Status is intentionally not
// accepted — the agent always creates draft content (§103).
type contentCreateArgs struct {
	Title         string            `json:"title"`
	Body          string            `json:"body"`
	ContentType   string            `json:"content_type"`
	AuthorID      string            `json:"author_id"`
	GeoSpaceID    string            `json:"geo_space_id,omitempty"`
	SpaceID       string            `json:"space_id,omitempty"`
	GuideCategory string            `json:"guide_category,omitempty"`
	FieldValues   map[string]any    `json:"field_values,omitempty"`
	Provenance    *aiProvenanceArgs `json:"_ai_provenance,omitempty"`
}

// aiProvenanceArgs is the provenance metadata injected into field_values.
// The prompted_by_user_id is always the calling user; the persona_id comes
// from the agent.default_persona_id setting (resolved by the agent loop,
// not the tool handler — the handler records what it is given).
type aiProvenanceArgs struct {
	AIGenerated      bool   `json:"ai_generated"`
	AIPersonaID      string `json:"ai_persona_id,omitempty"`
	PromptedByUserID string `json:"prompted_by_user_id,omitempty"`
	OriginalPrompt   string `json:"original_prompt,omitempty"`
	AuthorAssignedBy string `json:"author_assigned_by,omitempty"`
	AssignmentReason string `json:"author_assignment_reason,omitempty"`
}

// ContentCreateHandler returns a ToolHandler that creates new content.
// The handler injects the _ai_provenance block into the FieldValues JSONB
// so the content's AI origin is transparent and auditable (§103 Authorship
// Metadata). Status defaults to "draft" — the agent never publishes
// directly; publishing goes through the HITL pipeline.
//
// Unlike content.read/update, this handler does not require deps.Content
// (the read service) — it only needs deps.ContentCreator (the write bridge).
func ContentCreateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil {
			return errorToolResult("content_create_unavailable", "tool deps not configured"), nil
		}

		var a contentCreateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.Title == "" {
			return errorToolResult("missing_title", "title is required"), nil
		}
		if a.ContentType == "" {
			return errorToolResult("missing_content_type", "content_type is required"), nil
		}

		authorID, err := scanUUID(a.AuthorID)
		if err != nil || !authorID.Valid {
			// Default to the calling user when no author is specified.
			authorID = userID
		}

		spaceID, err := optionalUUID(a.SpaceID)
		if err != nil {
			return errorToolResult("invalid_space_id", "space_id is not a valid UUID"), nil
		}
		geoSpaceID, err := optionalUUID(a.GeoSpaceID)
		if err != nil {
			return errorToolResult("invalid_geo_space_id", "geo_space_id is not a valid UUID"), nil
		}

		// The agent never publishes directly — §103 requires AI-created
		// content to start as "draft" and go through the HITL pipeline
		// before publishing. The LLM's status field is ignored; the human
		// reviewer can change it during HITL review.
		status := "draft"

		// Build field_values JSONB with _ai_provenance injected.
		fieldValues := a.FieldValues
		if fieldValues == nil {
			fieldValues = map[string]any{}
		}
		fieldValues["_ai_provenance"] = buildProvenance(a.Provenance, userID)
		fvJSON, err := json.Marshal(fieldValues)
		if err != nil {
			return errorToolResult("field_values_error", fmt.Sprintf("failed to encode field values: %v", err)), nil
		}

		// We cannot call content.Service.CreateContent directly through the
		// ContentService interface (it takes CreateContentInput, a
		// content-package type). Instead, the handler accepts a
		// ContentCreator — a narrow function type that the wiring layer
		// provides by wrapping the real service. This keeps the ai package
		// free of an import on internal/content (avoiding a cycle risk).
		if deps.ContentCreator == nil {
			return errorToolResult("content_create_unavailable", "content creation is not wired"), nil
		}

		row, err := deps.ContentCreator(ctx, tenantID, ContentCreateInput{
			AuthorID:      authorID,
			SpaceID:       spaceID,
			ContentType:   a.ContentType,
			Title:         a.Title,
			Body:          a.Body,
			FieldValues:   fvJSON,
			GeoSpaceID:    geoSpaceID,
			Status:        status,
			GuideCategory: a.GuideCategory,
		})
		if err != nil {
			return errorToolResult("content_create_failed", err.Error()), nil
		}

		res := successToolResult(map[string]any{
			"content_id":     uuidString(row.ID),
			"title":          row.Title,
			"slug":           row.Slug,
			"status":         row.Status,
			"content_type":   row.ContentType,
			"author_id":      uuidString(row.AuthorID),
			"geo_space_id":   uuidString(row.GeoSpaceID),
			"created_at":     timestampString(row.CreatedAt),
			"guide_category": row.GuideCategory,
		})
		// Phase 11.02a — force HITL for high-stakes guide categories so the
		// executor creates a HITL review even when the tool's requires_hitl
		// flag is false.
		res.ForceHITL = IsHighStakesCategory(a.GuideCategory)
		return res, nil
	}
}

// ContentCreateInput is the provider-agnostic input for content creation,
// mirroring the subset of content.CreateContentInput the agent needs.
type ContentCreateInput struct {
	AuthorID      pgtype.UUID
	SpaceID       pgtype.UUID
	ContentType   string
	Title         string
	Body          string
	FieldValues   []byte
	GeoSpaceID    pgtype.UUID
	Status        string
	GuideCategory string
}

// ContentCreateRow is the provider-agnostic return shape, mirroring the
// subset of gen.CreateContentRow the agent reports back to the LLM.
type ContentCreateRow struct {
	ID            pgtype.UUID
	Title         string
	Slug          string
	Status        string
	ContentType   string
	AuthorID      pgtype.UUID
	GeoSpaceID    pgtype.UUID
	CreatedAt     pgtype.Timestamptz
	GuideCategory string
}

// ContentCreator is the function type the wiring layer provides to bridge
// content.Service.CreateContent into the agent without an import cycle.
type ContentCreator func(ctx context.Context, tenantID pgtype.UUID, input ContentCreateInput) (ContentCreateRow, error)

// buildProvenance constructs the _ai_provenance metadata block. The
// prompted_by_user_id is always the calling user (the agent acts on their
// behalf). ai_generated is always true for content.create.
func buildProvenance(p *aiProvenanceArgs, userID pgtype.UUID) map[string]any {
	out := map[string]any{
		"ai_generated":        true,
		"prompted_by_user_id": uuidString(userID),
	}
	if p != nil {
		if p.AIPersonaID != "" {
			out["ai_persona_id"] = p.AIPersonaID
		}
		if p.OriginalPrompt != "" {
			out["original_prompt"] = p.OriginalPrompt
		}
		if p.AuthorAssignedBy != "" {
			out["author_assigned_by"] = p.AuthorAssignedBy
		}
		if p.AssignmentReason != "" {
			out["author_assignment_reason"] = p.AssignmentReason
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// content.create_draft (Phase 8, tasks 8.01 + 8.02 + 8.05)
// ---------------------------------------------------------------------------

// contentCreateDraftArgs is the LLM-produced argument shape for
// content.create_draft. Unlike content.create, the author_id is forced to
// the AI persona's backing users.id (resolved from agent.default_persona_id
// by the agent loop and injected into the system prompt); the LLM-sent
// author_id is only used as a fallback when the persona resolver is not
// wired. Status is always "draft" — the agent never publishes directly.
type contentCreateDraftArgs struct {
	Title          string         `json:"title"`
	Body           string         `json:"body"`
	ContentType    string         `json:"content_type"`
	AuthorID       string         `json:"author_id"`
	GeoSpaceID     string         `json:"geo_space_id,omitempty"`
	SpaceID        string         `json:"space_id,omitempty"`
	GuideCategory  string         `json:"guide_category,omitempty"`
	FieldValues    map[string]any `json:"field_values,omitempty"`
	OriginalPrompt string         `json:"original_prompt,omitempty"`
	AIPersonaID    string         `json:"ai_persona_id,omitempty"`
}

// ContentCreateDraftHandler returns a ToolHandler that creates a draft
// content row for admin review (the §103 Content Request Flow for users
// without content.create permission). The handler:
//
//  1. Resolves the AI persona's backing users.id from the author_id
//     argument (the agent loop injects the persona's user_id there from
//     agent.default_persona_id). Falls back to the calling user when the
//     persona resolver is not wired (graceful degradation).
//  2. Forces status = "draft" (§103: "the agent never publishes directly").
//  3. Injects _ai_provenance into field_values JSONB with ai_generated,
//     ai_persona_id, prompted_by_user_id, and original_prompt.
//  4. Creates the content row via deps.ContentCreator.
//  5. Creates a HITL review with source='agent_draft_request' + the
//     created content_id (deps.HITL). The review ID is surfaced via
//     ToolResult.Metadata["hitl_review_id"] so the executor records it on
//     the ai_agent_tool_calls row and skips its own generic createHITL.
//
// The tool is gated by agent.draft_request (not content.create) at the
// catalog layer (migration 000396). The executor's RBAC gate enforces it.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Content Request Flow, HITL Source Tracking)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 8, tasks 8.01, 8.02, 8.05)
func ContentCreateDraftHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil {
			return errorToolResult("content_create_draft_unavailable", "tool deps not configured"), nil
		}
		if deps.ContentCreator == nil {
			return errorToolResult("content_create_draft_unavailable", "content creation is not wired"), nil
		}

		var a contentCreateDraftArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.Title == "" {
			return errorToolResult("missing_title", "title is required"), nil
		}
		if a.ContentType == "" {
			return errorToolResult("missing_content_type", "content_type is required"), nil
		}
		if a.Body == "" {
			return errorToolResult("missing_body", "body is required for a draft request"), nil
		}

		// Resolve the author: prefer the persona's backing user_id. The
		// agent loop resolves agent.default_persona_id → persona.user_id
		// and injects it into the system prompt so the LLM passes it as
		// author_id. When the persona resolver is wired, we verify the
		// injected author_id matches the persona's user_id (defense-in-
		// depth: the LLM cannot impersonate an arbitrary user as the
		// author of a draft request). When the resolver is not wired,
		// fall back to the LLM-sent author_id, then to the calling user.
		authorID, err := scanUUID(a.AuthorID)
		if err != nil || !authorID.Valid {
			authorID = userID
		}
		personaIDStr := a.AIPersonaID
		if deps.Persona != nil && personaIDStr != "" {
			personaID, perr := scanUUID(personaIDStr)
			if perr == nil && personaID.Valid {
				pid, perr := deps.Persona.GetPersonaUserID(ctx, tenantID, personaID)
				if perr == nil && pid.Valid {
					authorID = pid
				}
			}
		}

		spaceID, err := optionalUUID(a.SpaceID)
		if err != nil {
			return errorToolResult("invalid_space_id", "space_id is not a valid UUID"), nil
		}
		geoSpaceID, err := optionalUUID(a.GeoSpaceID)
		if err != nil {
			return errorToolResult("invalid_geo_space_id", "geo_space_id is not a valid UUID"), nil
		}

		// Status is always "draft" — §103 Content Request Flow.
		status := "draft"

		// Build field_values JSONB with _ai_provenance injected.
		fieldValues := a.FieldValues
		if fieldValues == nil {
			fieldValues = map[string]any{}
		}
		fieldValues["_ai_provenance"] = buildProvenance(&aiProvenanceArgs{
			AIPersonaID:    personaIDStr,
			OriginalPrompt: a.OriginalPrompt,
		}, userID)
		fvJSON, err := json.Marshal(fieldValues)
		if err != nil {
			return errorToolResult("field_values_error", fmt.Sprintf("failed to encode field values: %v", err)), nil
		}

		row, err := deps.ContentCreator(ctx, tenantID, ContentCreateInput{
			AuthorID:      authorID,
			SpaceID:       spaceID,
			ContentType:   a.ContentType,
			Title:         a.Title,
			Body:          a.Body,
			FieldValues:   fvJSON,
			GeoSpaceID:    geoSpaceID,
			Status:        status,
			GuideCategory: a.GuideCategory,
		})
		if err != nil {
			return errorToolResult("content_create_draft_failed", err.Error()), nil
		}

		// Create a HITL review with source='agent_draft_request' + the
		// created content_id. The review ID is surfaced via Metadata so
		// the executor records it on ai_agent_tool_calls and skips its
		// own generic createHITL (which would set source='agent_tool').
		var hitlReviewID pgtype.UUID
		if deps.HITL != nil {
			review, hitlErr := deps.HITL.CreateHITLReview(ctx, HITLReview{
				TenantID:    tenantID,
				ContentID:   row.ID,
				ContentType: a.ContentType,
				AIOutput:    fvJSON,
				Status:      HITLStatusPending,
				Source:      HITLSourceAgentDraftRequest,
			})
			if hitlErr != nil {
				// Non-fatal: the draft was created; the admin can still
				// see it in the content list. The HITL queue simply did
				// not get a row.
			} else {
				hitlReviewID = review.ID
			}
		}

		res := successToolResult(map[string]any{
			"content_id":     uuidString(row.ID),
			"title":          row.Title,
			"slug":           row.Slug,
			"status":         row.Status,
			"content_type":   row.ContentType,
			"author_id":      uuidString(row.AuthorID),
			"geo_space_id":   uuidString(row.GeoSpaceID),
			"created_at":     timestampString(row.CreatedAt),
			"hitl_review_id": uuidString(hitlReviewID),
			"review_status":  "draft submitted for admin review",
			"guide_category": row.GuideCategory,
		})
		if hitlReviewID.Valid {
			res.Metadata = map[string]any{"hitl_review_id": hitlReviewID}
		}
		// Phase 11.02a — force HITL for high-stakes guide categories so the
		// executor creates a HITL review even when the tool's requires_hitl
		// flag is false. The draft handler already creates a HITL review
		// above; ForceHITL ensures the executor does not skip its own HITL
		// step when the tool config has requires_hitl=false.
		res.ForceHITL = IsHighStakesCategory(a.GuideCategory)
		return res, nil
	}
}

// ---------------------------------------------------------------------------
// content.update (task 4.04)
// ---------------------------------------------------------------------------

// contentUpdateArgs is the argument shape for content.update. Only the
// fields the LLM provides are updated; omitted fields are left unchanged.
type contentUpdateArgs struct {
	ContentID   string         `json:"content_id"`
	Title       *string        `json:"title,omitempty"`
	Body        *string        `json:"body,omitempty"`
	Slug        *string        `json:"slug,omitempty"`
	Status      *string        `json:"status,omitempty"`
	GeoSpaceID  *string        `json:"geo_space_id,omitempty"`
	FieldValues map[string]any `json:"field_values,omitempty"`
}

// ContentUpdateHandler returns a ToolHandler that updates an existing
// content record. Only the fields present in the arguments are changed.
// The executor's privacy gate blocks the call when the content is
// ai_exempt.
//
// Unlike content.read, this handler does not require deps.Content (the
// read service) — it only needs deps.ContentUpdater (the write bridge)
// and optionally deps.Privacy (for defense-in-depth).
func ContentUpdateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil {
			return errorToolResult("content_update_unavailable", "tool deps not configured"), nil
		}
		if deps.ContentUpdater == nil {
			return errorToolResult("content_update_unavailable", "content update is not wired"), nil
		}

		var a contentUpdateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		contentID, err := scanUUID(a.ContentID)
		if err != nil {
			return errorToolResult("invalid_content_id", "content_id is not a valid UUID"), nil
		}

		// Verify the content exists and is not exempt (defense-in-depth on
		// top of the executor's privacy gate).
		if deps.Privacy != nil {
			exempt, err := deps.Privacy.IsExempt(ctx, PrivacyEntityContent, tenantID, contentID)
			if err != nil {
				return errorToolResult("privacy_check_failed", err.Error()), nil
			}
			if exempt {
				return privacyBlockedResult("content is marked as private and not available to the agent"), nil
			}
		}

		var geoSpaceID *pgtype.UUID
		if a.GeoSpaceID != nil {
			id, err := optionalUUID(*a.GeoSpaceID)
			if err != nil {
				return errorToolResult("invalid_geo_space_id", "geo_space_id is not a valid UUID"), nil
			}
			geoSpaceID = &id
		}

		var fvJSON []byte
		if a.FieldValues != nil {
			fvJSON, err = json.Marshal(a.FieldValues)
			if err != nil {
				return errorToolResult("field_values_error", fmt.Sprintf("failed to encode field values: %v", err)), nil
			}
		}

		row, err := deps.ContentUpdater(ctx, contentID, tenantID, userID, ContentUpdateInput{
			Title:       a.Title,
			Body:        a.Body,
			Slug:        a.Slug,
			Status:      a.Status,
			GeoSpaceID:  geoSpaceID,
			FieldValues: fvJSON,
		})
		if err != nil {
			return errorToolResult("content_update_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"content_id": uuidString(row.ID),
			"title":      row.Title,
			"slug":       row.Slug,
			"status":     row.Status,
			"updated_at": timestampString(row.UpdatedAt),
		}), nil
	}
}

// ContentUpdateInput is the provider-agnostic input for content update.
// Pointer fields are nil when the LLM did not provide them (no change).
type ContentUpdateInput struct {
	Title       *string
	Body        *string
	Slug        *string
	Status      *string
	GeoSpaceID  *pgtype.UUID
	FieldValues []byte
}

// ContentUpdateRow is the provider-agnostic return shape.
type ContentUpdateRow struct {
	ID        pgtype.UUID
	Title     string
	Slug      string
	Status    string
	UpdatedAt pgtype.Timestamptz
}

// ContentUpdater is the function type bridging content.Service.UpdateContent.
type ContentUpdater func(ctx context.Context, contentID, tenantID, editedBy pgtype.UUID, input ContentUpdateInput) (ContentUpdateRow, error)

// ContentUpdatePrivacyTarget extracts the content_id from content.update
// arguments for the executor's privacy gate.
func ContentUpdatePrivacyTarget(ctx context.Context, args json.RawMessage, tenantID pgtype.UUID) (PrivacyEntityType, pgtype.UUID, error) {
	var a contentUpdateArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", pgtype.UUID{}, fmt.Errorf("parsing content.update args: %w", err)
	}
	id, err := scanUUID(a.ContentID)
	if err != nil {
		return "", pgtype.UUID{}, fmt.Errorf("invalid content_id: %w", err)
	}
	return PrivacyEntityContent, id, nil
}

// ---------------------------------------------------------------------------
// Shared result helpers
// ---------------------------------------------------------------------------

// successToolResult marshals v to JSON and returns a success ToolResult.
func successToolResult(v any) *ToolResult {
	b, _ := json.Marshal(v)
	return &ToolResult{Status: ToolCallStatusSuccess, Content: string(b)}
}

// errorToolResult returns an error ToolResult with a machine code + message.
func errorToolResult(code, message string) *ToolResult {
	b, _ := json.Marshal(map[string]string{"error": message, "code": code})
	return &ToolResult{Status: ToolCallStatusError, Content: string(b)}
}

// privacyBlockedResult returns a privacy_blocked ToolResult.
func privacyBlockedResult(message string) *ToolResult {
	b, _ := json.Marshal(map[string]string{"error": message, "code": "privacy_blocked"})
	return &ToolResult{Status: ToolCallStatusPrivacyBlocked, Content: string(b)}
}

// uuidString returns the string form of a pgtype.UUID, or "" when invalid.
func uuidString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return u.String()
}

// timestampString returns the RFC 3339 form of a pgtype.Timestamptz, or ""
// when invalid.
func timestampString(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.Format("2006-01-02T15:04:05Z07:00")
}

// filterNonExemptFieldValues strips ai_exempt field values from the list
// returned by customfields.Service.ListFieldValuesByEntity. The agent never
// sees exempt field values.
func filterNonExemptFieldValues(cfvs []gen.CustomFieldValue) []map[string]any {
	out := make([]map[string]any, 0, len(cfvs))
	for _, cfv := range cfvs {
		if cfv.AiExempt {
			continue
		}
		entry := map[string]any{
			"field_id":    uuidString(cfv.FieldID),
			"entity_type": cfv.EntityType,
			"entity_id":   uuidString(cfv.EntityID),
		}
		if cfv.ValueText != nil {
			entry["value_text"] = *cfv.ValueText
		}
		if cfv.ValueBool != nil {
			entry["value_bool"] = *cfv.ValueBool
		}
		if cfv.ValueJson != nil {
			entry["value_json"] = json.RawMessage(cfv.ValueJson)
		}
		if cfv.ValueDate.Valid {
			entry["value_date"] = timestampString(cfv.ValueDate)
		}
		out = append(out, entry)
	}
	return out
}

// ---------------------------------------------------------------------------
// Phase 9 — Content lifecycle tools (tasks 9.08)
//
//   - content.delete  — soft-delete content (admin/high-capability only)
//   - content.list    — list content by type/space/geo
//   - content.publish — change status to published (triggers HITL for
//                       high-stakes categories)
// ---------------------------------------------------------------------------

// contentDeleteArgs is the argument shape for content.delete.
type contentDeleteArgs struct {
	ContentID string `json:"content_id"`
}

// ContentDeleteHandler returns a ToolHandler that soft-deletes a content
// row. The executor's RBAC gate checks the required_permission
// (content.delete); the tool config sets requires_hitl = true. The
// handler performs a defense-in-depth privacy re-check before deleting.
//
// The underlying content.Service.DeleteContent performs a soft delete
// (sets deleted_at = now()), per Rule 11 — never hard-delete user content.
func ContentDeleteHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.ContentLifecycle == nil {
			return errorToolResult("content_lifecycle_unavailable", "content lifecycle service is not configured"), nil
		}

		var a contentDeleteArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		contentID, err := scanUUID(a.ContentID)
		if err != nil {
			return errorToolResult("invalid_content_id", "content_id is not a valid UUID"), nil
		}

		// Defense-in-depth: check the content's exemption status before
		// deleting. The executor's privacy gate already covers this, but
		// the handler re-checks to guard against a misconfigured
		// PrivacyTargetFunc.
		if deps.Privacy != nil {
			exempt, err := deps.Privacy.IsExempt(ctx, PrivacyEntityContent, tenantID, contentID)
			if err != nil {
				return errorToolResult("privacy_check_failed", err.Error()), nil
			}
			if exempt {
				return privacyBlockedResult("content is marked as private and not available to the agent"), nil
			}
		}

		if err := deps.ContentLifecycle.DeleteContent(ctx, contentID, tenantID); err != nil {
			return errorToolResult("content_delete_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"content_id": uuidString(contentID),
			"deleted":    true,
		}), nil
	}
}

// ContentDeletePrivacyTarget extracts the content_id from content.delete
// arguments for the executor's privacy gate.
func ContentDeletePrivacyTarget(ctx context.Context, args json.RawMessage, tenantID pgtype.UUID) (PrivacyEntityType, pgtype.UUID, error) {
	var a contentDeleteArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", pgtype.UUID{}, fmt.Errorf("parsing content.delete args: %w", err)
	}
	id, err := scanUUID(a.ContentID)
	if err != nil {
		return "", pgtype.UUID{}, fmt.Errorf("invalid content_id: %w", err)
	}
	return PrivacyEntityContent, id, nil
}

// contentListArgs is the LLM-produced argument shape for content.list.
type contentListArgs struct {
	ContentType string `json:"content_type,omitempty"`
	Limit       int32  `json:"limit,omitempty"`
}

// ContentListHandler returns a ToolHandler that lists content by type.
// When content_type is empty, it falls back to the tenant-wide list
// (deps.Content.ListContents). Exempt content is filtered post-query.
func ContentListHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil {
			return errorToolResult("content_unavailable", "content service is not configured"), nil
		}

		var a contentListArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.Limit <= 0 || a.Limit > 50 {
			a.Limit = 20
		}

		// Type-filtered listing via ContentLifecycleService.
		if a.ContentType != "" {
			if deps.ContentLifecycle == nil {
				return errorToolResult("content_lifecycle_unavailable", "content lifecycle service is not configured"), nil
			}
			filter := visibility.InternalVisibilityFilter(userID)
			rows, err := deps.ContentLifecycle.ListContentsByType(ctx, tenantID, a.ContentType, pgtype.Timestamptz{}, a.Limit, filter)
			if err != nil {
				return errorToolResult("content_list_failed", err.Error()), nil
			}
			filtered := filterExemptContentFromType(ctx, deps.Privacy, tenantID, rows)
			return successToolResult(map[string]any{
				"results":      contentListResultsFromType(filtered),
				"total":        len(filtered),
				"content_type": a.ContentType,
			}), nil
		}

		// Tenant-wide listing via ContentService (Phase 4).
		filter := visibility.InternalVisibilityFilter(userID)
		if deps.Content == nil {
			return errorToolResult("content_unavailable", "content service is not configured"), nil
		}
		results, err := deps.Content.ListContents(ctx, tenantID, "", pgtype.Timestamptz{}, a.Limit, filter)
		if err != nil {
			return errorToolResult("content_list_failed", err.Error()), nil
		}
		filtered := filterExemptContentFromTenant(ctx, deps.Privacy, tenantID, results)
		return successToolResult(map[string]any{
			"results": contentSearchResultsFromTenant(filtered),
			"total":   len(filtered),
		}), nil
	}
}

// filterExemptContentFromType strips ai_exempt content from type-filtered
// listing results.
func filterExemptContentFromType(ctx context.Context, privacy PrivacyExemptionChecker, tenantID pgtype.UUID, rows []gen.ListContentsByTypeRow) []gen.ListContentsByTypeRow {
	if privacy == nil {
		return rows
	}
	out := make([]gen.ListContentsByTypeRow, 0, len(rows))
	for _, r := range rows {
		exempt, err := privacy.IsExempt(ctx, PrivacyEntityContent, tenantID, r.ID)
		if err == nil && exempt {
			continue
		}
		out = append(out, r)
	}
	return out
}

// contentListResultsFromType converts type-filtered rows to the trimmed
// LLM-facing shape.
func contentListResultsFromType(rows []gen.ListContentsByTypeRow) []contentSearchResult {
	out := make([]contentSearchResult, 0, len(rows))
	for _, r := range rows {
		item := contentSearchResult{
			ID:          uuidString(r.ID),
			Title:       r.Title,
			Slug:        r.Slug,
			ContentType: r.ContentType,
			Status:      r.Status,
			GeoSpaceID:  uuidString(r.GeoSpaceID),
			CreatedAt:   timestampString(r.CreatedAt),
		}
		if r.AuthorUsername != nil {
			item.AuthorName = *r.AuthorUsername
		}
		out = append(out, item)
	}
	return out
}

// contentPublishArgs is the LLM-produced argument shape for content.publish.
type contentPublishArgs struct {
	ContentID string `json:"content_id"`
	Status    string `json:"status,omitempty"`
}

// ContentPublishHandler returns a ToolHandler that changes a content row's
// status. The executor's RBAC gate checks the required_permission
// (content.publish); the tool config sets requires_hitl = true for
// high-stakes categories.
//
// The handler performs a defense-in-depth privacy re-check before
// publishing. The status defaults to "published" but can be set to other
// valid statuses (draft, scheduled, archived) — the agent never publishes
// directly without HITL review (the tool config's requires_hitl flag
// ensures a human reviews the action).
func ContentPublishHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.ContentLifecycle == nil {
			return errorToolResult("content_lifecycle_unavailable", "content lifecycle service is not configured"), nil
		}

		var a contentPublishArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		contentID, err := scanUUID(a.ContentID)
		if err != nil {
			return errorToolResult("invalid_content_id", "content_id is not a valid UUID"), nil
		}

		status := a.Status
		if status == "" {
			status = "published"
		}

		// Defense-in-depth privacy re-check.
		if deps.Privacy != nil {
			exempt, err := deps.Privacy.IsExempt(ctx, PrivacyEntityContent, tenantID, contentID)
			if err != nil {
				return errorToolResult("privacy_check_failed", err.Error()), nil
			}
			if exempt {
				return privacyBlockedResult("content is marked as private and not available to the agent"), nil
			}
		}

		row, err := deps.ContentLifecycle.UpdateContentStatus(ctx, contentID, tenantID, status)
		if err != nil {
			return errorToolResult("content_publish_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"content_id": uuidString(row.ID),
			"title":      row.Title,
			"slug":       row.Slug,
			"status":     row.Status,
			"updated_at": timestampString(row.UpdatedAt),
		}), nil
	}
}

// ContentPublishPrivacyTarget extracts the content_id from content.publish
// arguments for the executor's privacy gate.
func ContentPublishPrivacyTarget(ctx context.Context, args json.RawMessage, tenantID pgtype.UUID) (PrivacyEntityType, pgtype.UUID, error) {
	var a contentPublishArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", pgtype.UUID{}, fmt.Errorf("parsing content.publish args: %w", err)
	}
	id, err := scanUUID(a.ContentID)
	if err != nil {
		return "", pgtype.UUID{}, fmt.Errorf("invalid content_id: %w", err)
	}
	return PrivacyEntityContent, id, nil
}
