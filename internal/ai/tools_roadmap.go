// Package ai — tools_roadmap.go implements the §113 public-roadmap admin
// surface for the agent catalog: operators and service agents (changelog
// generation, release automation, and §119 MCP callers) can list, read,
// create, update, and lifecycle-drive roadmap items under the same RBAC
// nodes as the /admin/roadmap/* REST routes — the catalog's
// required_permission column carries the gate, exactly as if the caller
// hit the REST surface directly.
//
// Tools implemented here:
//
//   - roadmap.categories.list  category catalog for item mapping
//   - roadmap.items.list       filtered/paginated admin item list (dedupe)
//   - roadmap.item.get         full item incl. body + deleted_at
//   - roadmap.item.create      new item; status="shipped" lands on the
//     changelog immediately
//   - roadmap.item.update      patch item fields
//   - roadmap.item.set_status  lifecycle state machine; → shipped stamps
//     shipped_at + enqueues roadmap:ship_notify
//
// No privacy targets are registered — roadmap items are platform data,
// not user content, so the ai_exempt gate does not apply.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Tool Catalog)
// Ref: japaconnect-engineering/sections/113-public-product-roadmap.md
// Ref: japaconnect-engineering/sections/119-mcp-server.md
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5/pgtype"
)

// roadmapListLimitCap bounds a single roadmap.items.list page — mirrors the
// admin DataTable page-size ceiling so an agent cannot pull the entire
// board in one call.
const roadmapListLimitCap = 100

// roadmapErrResult unwraps a RoadmapServiceError into its machine code and
// falls back to a generic code for unexpected adapter failures.
func roadmapErrResult(fallback string, err error) *ToolResult {
	var rerr *RoadmapServiceError
	if errors.As(err, &rerr) {
		return errorToolResult(rerr.Code, rerr.Message)
	}

	return errorToolResult(fallback, err.Error())
}

// roadmapItemArgs is the shared argument shape for create/update —
// pointer fields distinguish "absent" (keep current / default) from
// "set", matching the REST adminItemRequest contract.
type roadmapItemArgs struct {
	ID             *string `json:"id"`
	Title          *string `json:"title"`
	Slug           *string `json:"slug"`
	Summary        *string `json:"summary"`
	Body           *string `json:"body"`
	CategoryID     *string `json:"category_id"`
	Status         *string `json:"status"`
	SortOrder      *int32  `json:"sort_order"`
	IsFlagship     *bool   `json:"is_flagship"`
	TargetWindow   *string `json:"target_window"`
	FeatureFlagKey *string `json:"feature_flag_key"`
}

// toInput converts decoded tool args into a RoadmapItemInput. A non-nil
// empty category_id string clears the category (zero/invalid UUID),
// matching the REST toInput semantics.
func (a roadmapItemArgs) toInput(actorID pgtype.UUID) (RoadmapItemInput, error) {
	out := RoadmapItemInput{ActorID: actorID}

	if a.Title != nil {
		out.Title = *a.Title
	}

	if a.Slug != nil {
		out.Slug = *a.Slug
	}

	out.Summary = a.Summary
	out.Body = a.Body

	if a.CategoryID != nil {
		var categoryID pgtype.UUID
		if *a.CategoryID != "" {
			if err := categoryID.Scan(*a.CategoryID); err != nil {
				return RoadmapItemInput{}, errors.New("invalid category_id")
			}
		}

		out.CategoryID = &categoryID
	}

	if a.Status != nil {
		out.Status = *a.Status
	}

	out.SortOrder = a.SortOrder
	out.IsFlagship = a.IsFlagship
	out.TargetWindow = a.TargetWindow
	out.FeatureFlagKey = a.FeatureFlagKey

	return out, nil
}

// ---------------------------------------------------------------------------
// roadmap.categories.list
// ---------------------------------------------------------------------------

// RoadmapCategoriesListHandler returns the roadmap category catalog so an
// agent can map a changelog entry to a category before create/update.
func RoadmapCategoriesListHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Roadmap == nil {
			return errorToolResult("roadmap_unavailable", "roadmap service is not configured"), nil
		}

		cats, err := deps.Roadmap.ListCategories(ctx)
		if err != nil {
			return roadmapErrResult("roadmap_categories_failed", err), nil
		}

		return successToolResult(map[string]any{"categories": cats}), nil
	}
}

// ---------------------------------------------------------------------------
// roadmap.items.list
// ---------------------------------------------------------------------------

type roadmapItemsListArgs struct {
	Status         string `json:"status"`
	Q              string `json:"q"`
	CategoryID     string `json:"category_id"`
	Sort           string `json:"sort"`
	Page           int32  `json:"page"`
	Limit          int32  `json:"limit"`
	IncludeDeleted bool   `json:"include_deleted"`
}

// RoadmapItemsListHandler lists roadmap items across every status — the
// dedupe read an agent runs before creating a changelog entry.
func RoadmapItemsListHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Roadmap == nil {
			return errorToolResult("roadmap_unavailable", "roadmap service is not configured"), nil
		}

		var a roadmapItemsListArgs
		if len(args) > 0 {
			if err := json.Unmarshal(args, &a); err != nil {
				return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
			}
		}

		f := RoadmapItemFilter{
			Status:         a.Status,
			Search:         a.Q,
			Sort:           a.Sort,
			IncludeDeleted: a.IncludeDeleted,
			PageSize:       a.Limit,
		}

		if f.PageSize <= 0 {
			f.PageSize = 20 // REST admin default (roadmap.DefaultPageSize)
		}

		if f.PageSize > roadmapListLimitCap {
			f.PageSize = roadmapListLimitCap
		}

		if a.Page > 1 {
			// int64 math — page*size must not wrap int32 into a negative
			// OFFSET (the REST parser clamps the same bound).
			off := int64(a.Page-1) * int64(f.PageSize)
			if off > math.MaxInt32 {
				off = math.MaxInt32
			}

			f.PageOffset = int32(off)
		}

		if a.CategoryID != "" {
			categoryID, err := scanUUID(a.CategoryID)
			if err != nil {
				return errorToolResult("invalid_category_id", "category_id is not a valid UUID"), nil
			}

			f.CategoryID = categoryID
		}

		page, err := deps.Roadmap.ListItems(ctx, f)
		if err != nil {
			return roadmapErrResult("roadmap_list_failed", err), nil
		}

		return successToolResult(page), nil
	}
}

// ---------------------------------------------------------------------------
// roadmap.item.get
// ---------------------------------------------------------------------------

type roadmapItemGetArgs struct {
	ID string `json:"id"`
}

// RoadmapItemGetHandler reads one item by UUID including soft-deleted
// rows — mirrors GET /admin/roadmap/items/{id}.
func RoadmapItemGetHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Roadmap == nil {
			return errorToolResult("roadmap_unavailable", "roadmap service is not configured"), nil
		}

		var a roadmapItemGetArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if a.ID == "" {
			return errorToolResult("missing_id", "id is required"), nil
		}

		id, err := scanUUID(a.ID)
		if err != nil {
			return errorToolResult("invalid_id", "id is not a valid UUID"), nil
		}

		item, err := deps.Roadmap.GetItem(ctx, id)
		if err != nil {
			return roadmapErrResult("roadmap_get_failed", err), nil
		}

		return successToolResult(item), nil
	}
}

// ---------------------------------------------------------------------------
// roadmap.item.create
// ---------------------------------------------------------------------------

// RoadmapItemCreateHandler creates a roadmap item. status="shipped" lands
// the entry directly on /roadmap/changelog (the service stamps shipped_at
// and enqueues roadmap:ship_notify) — the changelog has no separate store.
func RoadmapItemCreateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Roadmap == nil {
			return errorToolResult("roadmap_unavailable", "roadmap service is not configured"), nil
		}

		var a roadmapItemArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if a.Title == nil || *a.Title == "" {
			return errorToolResult("missing_title", "title is required"), nil
		}

		in, err := a.toInput(userID)
		if err != nil {
			return errorToolResult("invalid_category_id", "category_id is not a valid UUID"), nil
		}

		item, err := deps.Roadmap.CreateItem(ctx, in)
		if err != nil {
			return roadmapErrResult("roadmap_create_failed", err), nil
		}

		return successToolResult(item), nil
	}
}

// ---------------------------------------------------------------------------
// roadmap.item.update
// ---------------------------------------------------------------------------

// RoadmapItemUpdateHandler patches a roadmap item — mirrors
// PATCH /admin/roadmap/items/{id}: absent fields keep current values.
func RoadmapItemUpdateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Roadmap == nil {
			return errorToolResult("roadmap_unavailable", "roadmap service is not configured"), nil
		}

		var a roadmapItemArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if a.ID == nil || *a.ID == "" {
			return errorToolResult("missing_id", "id is required"), nil
		}

		// Status is collected in the shared args shape but PATCH semantics
		// deliberately do not apply it — §113 transitions only happen
		// through the state machine (AdminSetStatus / roadmap.moderate).
		// Failing loudly beats the REST surface's silent no-op, which would
		// leave an agent believing a transition it requested never ran.
		if a.Status != nil && *a.Status != "" {
			return errorToolResult("use_set_status", "status is not changed by roadmap.item.update — call roadmap.item.set_status"), nil
		}

		id, err := scanUUID(*a.ID)
		if err != nil {
			return errorToolResult("invalid_id", "id is not a valid UUID"), nil
		}

		in, err := a.toInput(userID)
		if err != nil {
			return errorToolResult("invalid_category_id", "category_id is not a valid UUID"), nil
		}

		item, err := deps.Roadmap.UpdateItem(ctx, id, in)
		if err != nil {
			return roadmapErrResult("roadmap_update_failed", err), nil
		}

		return successToolResult(item), nil
	}
}

// ---------------------------------------------------------------------------
// roadmap.item.set_status
// ---------------------------------------------------------------------------

type roadmapSetStatusArgs struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// RoadmapItemSetStatusHandler drives the §113 lifecycle state machine —
// mirrors PATCH /admin/roadmap/items/{id}/status. Shipping stamps
// shipped_at, publishes the item to /roadmap/changelog, and notifies
// voters; flag-linked items reject with status_derived.
func RoadmapItemSetStatusHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Roadmap == nil {
			return errorToolResult("roadmap_unavailable", "roadmap service is not configured"), nil
		}

		var a roadmapSetStatusArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if a.ID == "" {
			return errorToolResult("missing_id", "id is required"), nil
		}

		if a.Status == "" {
			return errorToolResult("missing_status", "status is required"), nil
		}

		id, err := scanUUID(a.ID)
		if err != nil {
			return errorToolResult("invalid_id", "id is not a valid UUID"), nil
		}

		item, err := deps.Roadmap.SetItemStatus(ctx, id, a.Status, userID)
		if err != nil {
			return roadmapErrResult("roadmap_status_failed", err), nil
		}

		return successToolResult(item), nil
	}
}
