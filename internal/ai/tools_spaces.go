// Package ai — tools_spaces.go implements the space/geo tool handlers for
// the AI Agent Executor extended tool catalog (Section 103, Phase 9).
//
// Tools implemented here (Phase 9, tasks 9.01):
//
//   - spaces.find   — find a space by city name, geo code, or keywords
//   - spaces.read   — read a single space by ID
//   - spaces.list   — list spaces for the tenant (viewer-scoped)
//   - spaces.create — create a new space (admin, HITL)
//   - spaces.update — update an existing space (admin, HITL)
//
// Privacy: spaces are not user data and have no ai_exempt column, so no
// PrivacyTargetFunc is registered. The executor's RBAC gate enforces the
// required_permission on each tool (spaces.create / spaces.update require
// spaces.create / spaces.edit; the read tools are open to agent.use holders).
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Space/geo tools)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 9, task 9.01)
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// spaces.find (task 9.01a)
// ---------------------------------------------------------------------------

// spacesFindArgs is the LLM-produced argument shape for spaces.find. The
// handler tries city-name lookup, then geo-code lookup, then keyword
// lookup, and merges the results. At least one of the three must be
// provided.
type spacesFindArgs struct {
	CityName string   `json:"city_name,omitempty"`
	GeoCode  string   `json:"geo_code,omitempty"`
	Keywords []string `json:"keywords,omitempty"`
}

// SpacesFindHandler returns a ToolHandler that finds a space by city name,
// geo code, or keywords. The handler tries each lookup in order and
// returns the first match (or all keyword matches when keywords are used).
func SpacesFindHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Spaces == nil {
			return errorToolResult("spaces_unavailable", "spaces service is not configured"), nil
		}

		var a spacesFindArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if a.CityName == "" && a.GeoCode == "" && len(a.Keywords) == 0 {
			return errorToolResult("missing_query", "at least one of city_name, geo_code, or keywords is required"), nil
		}

		// 1. City-name lookup.
		if a.CityName != "" {
			space, err := deps.Spaces.FindCitySpaceByName(ctx, a.CityName)
			if err == nil {
				return successToolResult(map[string]any{
					"match_type": "city_name",
					"space":      spaceFindByCityRowToMap(space),
				}), nil
			}
			// Fall through to geo-code / keyword lookup on not-found.
		}

		// 2. Geo-code lookup.
		if a.GeoCode != "" {
			space, err := deps.Spaces.GetSpaceByGeoCode(ctx, a.GeoCode)
			if err == nil {
				return successToolResult(map[string]any{
					"match_type": "geo_code",
					"space":      spaceByGeoRowToMap(space),
				}), nil
			}
		}

		// 3. Keyword lookup.
		if len(a.Keywords) > 0 {
			spaces, err := deps.Spaces.FindSpacesByKeywords(ctx, a.Keywords)
			if err != nil {
				return errorToolResult("spaces_find_failed", err.Error()), nil
			}

			results := make([]map[string]any, 0, len(spaces))
			for _, s := range spaces {
				results = append(results, map[string]any{
					"id":    uuidString(s.ID),
					"name":  s.Name,
					"level": s.Level,
				})
			}

			return successToolResult(map[string]any{
				"match_type": "keywords",
				"spaces":     results,
				"total":      len(results),
			}), nil
		}

		return errorToolResult("space_not_found", "no space matched the provided criteria"), nil
	}
}

// ---------------------------------------------------------------------------
// spaces.read (task 9.01b)
// ---------------------------------------------------------------------------

// spacesReadArgs is the argument shape for spaces.read.
type spacesReadArgs struct {
	SpaceID string `json:"space_id"`
}

// SpacesReadHandler returns a ToolHandler that reads a single space by ID.
func SpacesReadHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Spaces == nil {
			return errorToolResult("spaces_unavailable", "spaces service is not configured"), nil
		}

		var a spacesReadArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		spaceID, err := scanUUID(a.SpaceID)
		if err != nil {
			return errorToolResult("invalid_space_id", "space_id is not a valid UUID"), nil
		}

		space, err := deps.Spaces.GetSpace(ctx, spaceID)
		if err != nil {
			return errorToolResult("space_not_found", fmt.Sprintf("space not found: %v", err)), nil
		}

		return successToolResult(map[string]any{
			"space": spaceRowToMap(space),
		}), nil
	}
}

// ---------------------------------------------------------------------------
// spaces.list (task 9.01c)
// ---------------------------------------------------------------------------

// SpacesListHandler returns a ToolHandler that lists spaces for the tenant.
// The list is viewer-scoped (the service filters by the viewer's membership
// and visibility rules).
func SpacesListHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Spaces == nil {
			return errorToolResult("spaces_unavailable", "spaces service is not configured"), nil
		}

		// No arguments — list all spaces visible to the calling user.
		spaces, err := deps.Spaces.ListSpaces(ctx, userID)
		if err != nil {
			return errorToolResult("spaces_list_failed", err.Error()), nil
		}

		results := make([]map[string]any, 0, len(spaces))
		for _, s := range spaces {
			results = append(results, spaceListRowToMap(s))
		}

		return successToolResult(map[string]any{
			"spaces": results,
			"total":  len(results),
		}), nil
	}
}

// ---------------------------------------------------------------------------
// spaces.create (task 9.01d)
// ---------------------------------------------------------------------------

// spacesCreateArgs is the LLM-produced argument shape for spaces.create.
type spacesCreateArgs struct {
	ParentID       string  `json:"parent_id,omitempty"`
	SpaceTypeID    string  `json:"space_type_id,omitempty"`
	Name           string  `json:"name"`
	Slug           string  `json:"slug"`
	Description    *string `json:"description,omitempty"`
	GeoCode        *string `json:"geo_code,omitempty"`
	Level          string  `json:"level"`
	Visibility     string  `json:"visibility,omitempty"`
	PostingPerm    string  `json:"posting_perm,omitempty"`
	ReplyingPerm   string  `json:"replying_perm,omitempty"`
	CommentingPerm string  `json:"commenting_perm,omitempty"`
	JoiningPerm    string  `json:"joining_perm,omitempty"`
	Status         string  `json:"status,omitempty"`
}

// SpacesCreateHandler returns a ToolHandler that creates a new space. The
// executor's RBAC gate checks the required_permission (spaces.create); the
// tool config sets requires_hitl = true so a human reviews the creation.
func SpacesCreateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Spaces == nil {
			return errorToolResult("spaces_unavailable", "spaces service is not configured"), nil
		}

		var a spacesCreateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if a.Name == "" {
			return errorToolResult("missing_name", "name is required"), nil
		}

		if a.Slug == "" {
			return errorToolResult("missing_slug", "slug is required"), nil
		}

		if a.Level == "" {
			return errorToolResult("missing_level", "level is required"), nil
		}

		parentID, err := optionalUUID(a.ParentID)
		if err != nil {
			return errorToolResult("invalid_parent_id", "parent_id is not a valid UUID"), nil
		}

		spaceTypeID, err := optionalUUID(a.SpaceTypeID)
		if err != nil {
			return errorToolResult("invalid_space_type_id", "space_type_id is not a valid UUID"), nil
		}

		// Default permissions/visibility/status when not provided.
		visibility := a.Visibility
		if visibility == "" {
			visibility = "public"
		}

		postingPerm := a.PostingPerm
		if postingPerm == "" {
			postingPerm = "members"
		}

		replyingPerm := a.ReplyingPerm
		if replyingPerm == "" {
			replyingPerm = "members"
		}

		commentingPerm := a.CommentingPerm
		if commentingPerm == "" {
			commentingPerm = "members"
		}

		joiningPerm := a.JoiningPerm
		if joiningPerm == "" {
			joiningPerm = "open"
		}

		status := a.Status
		if status == "" {
			status = "active"
		}

		space, err := deps.Spaces.CreateSpace(ctx, gen.CreateSpaceParams{

			ParentID:       parentID,
			SpaceTypeID:    spaceTypeID,
			Name:           a.Name,
			Slug:           a.Slug,
			Description:    a.Description,
			WikiContent:    []byte("null"),
			GeoCode:        a.GeoCode,
			Level:          a.Level,
			Visibility:     visibility,
			PostingPerm:    postingPerm,
			ReplyingPerm:   replyingPerm,
			CommentingPerm: commentingPerm,
			JoiningPerm:    joiningPerm,
			Status:         status,
		})
		if err != nil {
			return errorToolResult("space_create_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"space_id":   uuidString(space.ID),
			"name":       space.Name,
			"slug":       space.Slug,
			"level":      space.Level,
			"status":     space.Status,
			"visibility": space.Visibility,
			"created_at": timestampString(space.CreatedAt),
		}), nil
	}
}

// ---------------------------------------------------------------------------
// spaces.update (task 9.01e)
// ---------------------------------------------------------------------------

// spacesUpdateArgs is the LLM-produced argument shape for spaces.update.
type spacesUpdateArgs struct {
	SpaceID        string  `json:"space_id"`
	Name           string  `json:"name"`
	Description    *string `json:"description,omitempty"`
	GeoCode        *string `json:"geo_code,omitempty"`
	Visibility     string  `json:"visibility,omitempty"`
	PostingPerm    string  `json:"posting_perm,omitempty"`
	ReplyingPerm   string  `json:"replying_perm,omitempty"`
	CommentingPerm string  `json:"commenting_perm,omitempty"`
	JoiningPerm    string  `json:"joining_perm,omitempty"`
	Status         string  `json:"status,omitempty"`
}

// SpacesUpdateHandler returns a ToolHandler that updates an existing space.
// The executor's RBAC gate checks the required_permission (spaces.edit);
// the tool config sets requires_hitl = true.
func SpacesUpdateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Spaces == nil {
			return errorToolResult("spaces_unavailable", "spaces service is not configured"), nil
		}

		var a spacesUpdateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		spaceID, err := scanUUID(a.SpaceID)
		if err != nil {
			return errorToolResult("invalid_space_id", "space_id is not a valid UUID"), nil
		}

		if a.Name == "" {
			return errorToolResult("missing_name", "name is required"), nil
		}

		// Fetch existing to preserve fields not in the update args.
		existing, err := deps.Spaces.GetSpace(ctx, spaceID)
		if err != nil {
			return errorToolResult("space_not_found", fmt.Sprintf("space not found: %v", err)), nil
		}

		visibility := a.Visibility
		if visibility == "" {
			visibility = existing.Visibility
		}

		postingPerm := a.PostingPerm
		if postingPerm == "" {
			postingPerm = existing.PostingPerm
		}

		replyingPerm := a.ReplyingPerm
		if replyingPerm == "" {
			replyingPerm = existing.ReplyingPerm
		}

		commentingPerm := a.CommentingPerm
		if commentingPerm == "" {
			commentingPerm = existing.CommentingPerm
		}

		joiningPerm := a.JoiningPerm
		if joiningPerm == "" {
			joiningPerm = existing.JoiningPerm
		}

		status := a.Status
		if status == "" {
			status = existing.Status
		}

		description := a.Description
		if description == nil {
			description = existing.Description
		}

		geoCode := a.GeoCode
		if geoCode == nil {
			geoCode = existing.GeoCode
		}

		space, err := deps.Spaces.UpdateSpace(ctx, gen.UpdateSpaceParams{
			ID: spaceID,

			Name:           a.Name,
			Description:    description,
			WikiContent:    existing.WikiContent,
			GeoCode:        geoCode,
			Visibility:     visibility,
			PostingPerm:    postingPerm,
			ReplyingPerm:   replyingPerm,
			CommentingPerm: commentingPerm,
			JoiningPerm:    joiningPerm,
			Status:         status,
		})
		if err != nil {
			return errorToolResult("space_update_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"space_id":   uuidString(space.ID),
			"name":       space.Name,
			"slug":       space.Slug,
			"status":     space.Status,
			"updated_at": timestampString(space.UpdatedAt),
		}), nil
	}
}

// ---------------------------------------------------------------------------
// Shared row → map converters
// ---------------------------------------------------------------------------

// spaceRowToMap converts a gen.GetSpaceByIDRow to a JSON-safe map.
func spaceRowToMap(s gen.GetSpaceByIDRow) map[string]any {
	m := map[string]any{
		"id": uuidString(s.ID),

		"parent_id":       uuidString(s.ParentID),
		"space_type_id":   uuidString(s.SpaceTypeID),
		"name":            s.Name,
		"slug":            s.Slug,
		"level":           s.Level,
		"member_count":    s.MemberCount,
		"post_count":      s.PostCount,
		"follower_count":  s.FollowerCount,
		"visibility":      s.Visibility,
		"posting_perm":    s.PostingPerm,
		"replying_perm":   s.ReplyingPerm,
		"commenting_perm": s.CommentingPerm,
		"joining_perm":    s.JoiningPerm,
		"status":          s.Status,
		"created_at":      timestampString(s.CreatedAt),
		"updated_at":      timestampString(s.UpdatedAt),
	}
	if s.Description != nil {
		m["description"] = *s.Description
	}

	if s.GeoCode != nil {
		m["geo_code"] = *s.GeoCode
	}

	if s.CurrencyCode != nil {
		m["currency_code"] = *s.CurrencyCode
	}

	return m
}

// spaceByGeoRowToMap converts a gen.GetSpaceByGeoCodeRow to a JSON-safe map.
func spaceByGeoRowToMap(s gen.GetSpaceByGeoCodeRow) map[string]any {
	m := map[string]any{
		"id": uuidString(s.ID),

		"parent_id":       uuidString(s.ParentID),
		"space_type_id":   uuidString(s.SpaceTypeID),
		"name":            s.Name,
		"slug":            s.Slug,
		"level":           s.Level,
		"member_count":    s.MemberCount,
		"post_count":      s.PostCount,
		"follower_count":  s.FollowerCount,
		"visibility":      s.Visibility,
		"posting_perm":    s.PostingPerm,
		"replying_perm":   s.ReplyingPerm,
		"commenting_perm": s.CommentingPerm,
		"joining_perm":    s.JoiningPerm,
		"status":          s.Status,
		"created_at":      timestampString(s.CreatedAt),
		"updated_at":      timestampString(s.UpdatedAt),
	}
	if s.Description != nil {
		m["description"] = *s.Description
	}

	if s.GeoCode != nil {
		m["geo_code"] = *s.GeoCode
	}

	return m
}

// spaceFindByCityRowToMap converts a gen.FindCitySpaceByNameRow to a
// JSON-safe map. This row type has the same columns as GetSpaceByIDRow but
// is a distinct sqlc-generated type.
func spaceFindByCityRowToMap(s gen.FindCitySpaceByNameRow) map[string]any {
	m := map[string]any{
		"id": uuidString(s.ID),

		"parent_id":       uuidString(s.ParentID),
		"space_type_id":   uuidString(s.SpaceTypeID),
		"name":            s.Name,
		"slug":            s.Slug,
		"level":           s.Level,
		"member_count":    s.MemberCount,
		"post_count":      s.PostCount,
		"follower_count":  s.FollowerCount,
		"visibility":      s.Visibility,
		"posting_perm":    s.PostingPerm,
		"replying_perm":   s.ReplyingPerm,
		"commenting_perm": s.CommentingPerm,
		"joining_perm":    s.JoiningPerm,
		"status":          s.Status,
		"created_at":      timestampString(s.CreatedAt),
		"updated_at":      timestampString(s.UpdatedAt),
	}
	if s.Description != nil {
		m["description"] = *s.Description
	}

	if s.GeoCode != nil {
		m["geo_code"] = *s.GeoCode
	}

	return m
}

// spaceListRowToMap converts a gen.ListSpacesByTenantRow to a JSON-safe map.
func spaceListRowToMap(s gen.ListSpacesByTenantRow) map[string]any {
	m := map[string]any{
		"id":             uuidString(s.ID),
		"parent_id":      uuidString(s.ParentID),
		"space_type_id":  uuidString(s.SpaceTypeID),
		"name":           s.Name,
		"slug":           s.Slug,
		"level":          s.Level,
		"member_count":   s.MemberCount,
		"post_count":     s.PostCount,
		"follower_count": s.FollowerCount,
		"visibility":     s.Visibility,
		"status":         s.Status,
		"created_at":     timestampString(s.CreatedAt),
		"updated_at":     timestampString(s.UpdatedAt),
	}
	if s.Description != nil {
		m["description"] = *s.Description
	}

	if s.GeoCode != nil {
		m["geo_code"] = *s.GeoCode
	}

	return m
}
