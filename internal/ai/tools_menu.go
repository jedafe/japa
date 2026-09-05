// Package ai — tools_menu.go implements the menu tool handlers for the AI
// Agent Executor extended tool catalog (Section 103, Phase 9).
//
// Tools implemented here (Phase 9, tasks 9.05):
//
//   - menu.list   — list menus (with optional filters)
//   - menu.create — create a menu item (admin, HITL)
//   - menu.update — update a menu item (admin, HITL)
//   - menu.delete — delete a menu item (admin, HITL)
//
// Privacy: menus are not user data and have no ai_exempt column, so no
// PrivacyTargetFunc is registered. The executor's RBAC gate enforces the
// required_permission (menus.manage) on each tool.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Menu tools)
// Ref: japaconnect-engineering/sections/054-menu-system.md (ADR-054)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 9, task 9.05)
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// menu.list (task 9.05a)
// ---------------------------------------------------------------------------

// menuListArgs is the LLM-produced argument shape for menu.list.
type menuListArgs struct {
	Placement string `json:"placement,omitempty"`
	Status    string `json:"status,omitempty"`
	Query     string `json:"q,omitempty"`
	Limit     int32  `json:"limit,omitempty"`
	Offset    int32  `json:"offset,omitempty"`
}

// MenuListHandler returns a ToolHandler that lists menus with optional
// placement/status filters.
func MenuListHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Menus == nil {
			return errorToolResult("menus_unavailable", "menus service is not configured"), nil
		}

		var a menuListArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if a.Limit <= 0 || a.Limit > 100 {
			a.Limit = 20
		}

		resp, err := deps.Menus.ListMenus(ctx, tenantID, a.Query, a.Placement, a.Status, "created_at", "asc", a.Limit, a.Offset)
		if err != nil {
			return errorToolResult("menu_list_failed", err.Error()), nil
		}

		results := make([]map[string]any, 0, len(resp.Items))
		for _, m := range resp.Items {
			results = append(results, map[string]any{
				"id":         uuidString(m.ID),
				"slug":       m.Slug,
				"name":       m.Name,
				"placement":  m.Placement,
				"status":     m.Status,
				"item_count": m.ItemCount,
				"created_at": timestampString(m.CreatedAt),
				"updated_at": timestampString(m.UpdatedAt),
			})
		}

		return successToolResult(map[string]any{
			"menus":       results,
			"total":       resp.Total,
			"total_pages": resp.TotalPages,
			"page":        resp.Page,
		}), nil
	}
}

// ---------------------------------------------------------------------------
// menu.create (task 9.05b)
// ---------------------------------------------------------------------------

// menuCreateArgs is the LLM-produced argument shape for menu.create.
type menuCreateArgs struct {
	MenuID           string          `json:"menu_id"`
	ParentID         *string         `json:"parent_id,omitempty"`
	Label            string          `json:"label"`
	URL              string          `json:"url"`
	Icon             *string         `json:"icon,omitempty"`
	SortOrder        int             `json:"sort_order,omitempty"`
	PermissionNode   *string         `json:"permission_node,omitempty"`
	RequiredGroupIDs []string        `json:"required_group_ids,omitempty"`
	Visibility       string          `json:"visibility,omitempty"`
	Target           string          `json:"target,omitempty"`
	IsAdSlot         bool            `json:"is_ad_slot,omitempty"`
	AdSlotConfig     json.RawMessage `json:"ad_slot_config,omitempty"`
	Status           string          `json:"status,omitempty"`
}

// MenuCreateHandler returns a ToolHandler that creates a menu item. The
// executor's RBAC gate checks the required_permission (menus.manage); the
// tool config sets requires_hitl = true.
func MenuCreateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Menus == nil {
			return errorToolResult("menus_unavailable", "menus service is not configured"), nil
		}

		var a menuCreateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if a.MenuID == "" {
			return errorToolResult("missing_menu_id", "menu_id is required"), nil
		}

		if a.Label == "" {
			return errorToolResult("missing_label", "label is required"), nil
		}

		visibility := a.Visibility
		if visibility == "" {
			visibility = "all"
		}

		target := a.Target
		if target == "" {
			target = "_self"
		}

		item, err := deps.Menus.CreateMenuItem(ctx, tenantID, MenusCreateMenuItemInput{
			MenuID:           a.MenuID,
			ParentID:         a.ParentID,
			Label:            a.Label,
			URL:              a.URL,
			Icon:             a.Icon,
			SortOrder:        a.SortOrder,
			PermissionNode:   a.PermissionNode,
			RequiredGroupIDs: a.RequiredGroupIDs,
			Visibility:       visibility,
			Target:           target,
			IsAdSlot:         a.IsAdSlot,
			AdSlotConfig:     a.AdSlotConfig,
			Status:           a.Status,
		})
		if err != nil {
			return errorToolResult("menu_create_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"item_id":    uuidString(item.ID),
			"menu_id":    uuidString(item.MenuID),
			"label":      item.Label,
			"url":        item.Url,
			"visibility": item.Visibility,
			"target":     item.Target,
			"status":     item.Status,
			"created_at": timestampString(item.CreatedAt),
		}), nil
	}
}

// ---------------------------------------------------------------------------
// menu.update (task 9.05c)
// ---------------------------------------------------------------------------

// menuUpdateArgs is the LLM-produced argument shape for menu.update.
type menuUpdateArgs struct {
	ItemID           string          `json:"item_id"`
	Label            string          `json:"label"`
	URL              string          `json:"url,omitempty"`
	Icon             *string         `json:"icon,omitempty"`
	ParentID         *string         `json:"parent_id,omitempty"`
	SortOrder        int             `json:"sort_order,omitempty"`
	PermissionNode   *string         `json:"permission_node,omitempty"`
	RequiredGroupIDs []string        `json:"required_group_ids,omitempty"`
	Visibility       string          `json:"visibility,omitempty"`
	Target           string          `json:"target,omitempty"`
	IsAdSlot         bool            `json:"is_ad_slot,omitempty"`
	AdSlotConfig     json.RawMessage `json:"ad_slot_config,omitempty"`
	Status           string          `json:"status,omitempty"`
}

// MenuUpdateHandler returns a ToolHandler that updates a menu item. The
// executor's RBAC gate checks the required_permission (menus.manage); the
// tool config sets requires_hitl = true.
func MenuUpdateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Menus == nil {
			return errorToolResult("menus_unavailable", "menus service is not configured"), nil
		}

		var a menuUpdateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		itemID, err := scanUUID(a.ItemID)
		if err != nil {
			return errorToolResult("invalid_item_id", "item_id is not a valid UUID"), nil
		}

		if a.Label == "" {
			return errorToolResult("missing_label", "label is required"), nil
		}

		visibility := a.Visibility
		if visibility == "" {
			visibility = "all"
		}

		target := a.Target
		if target == "" {
			target = "_self"
		}

		status := a.Status
		if status == "" {
			status = "active"
		}

		item, err := deps.Menus.UpdateMenuItem(ctx, itemID, tenantID, MenusUpdateMenuItemInput{
			Label:            a.Label,
			URL:              a.URL,
			Icon:             a.Icon,
			ParentID:         a.ParentID,
			SortOrder:        a.SortOrder,
			PermissionNode:   a.PermissionNode,
			RequiredGroupIDs: a.RequiredGroupIDs,
			Visibility:       visibility,
			Target:           target,
			IsAdSlot:         a.IsAdSlot,
			AdSlotConfig:     a.AdSlotConfig,
			Status:           status,
		})
		if err != nil {
			return errorToolResult("menu_update_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"item_id":    uuidString(item.ID),
			"label":      item.Label,
			"status":     item.Status,
			"updated_at": timestampString(item.UpdatedAt),
		}), nil
	}
}

// ---------------------------------------------------------------------------
// menu.delete (task 9.05d)
// ---------------------------------------------------------------------------

// menuDeleteArgs is the argument shape for menu.delete.
type menuDeleteArgs struct {
	ItemID string `json:"item_id"`
}

// MenuDeleteHandler returns a ToolHandler that deletes a menu item. The
// executor's RBAC gate checks the required_permission (menus.manage); the
// tool config sets requires_hitl = true.
func MenuDeleteHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Menus == nil {
			return errorToolResult("menus_unavailable", "menus service is not configured"), nil
		}

		var a menuDeleteArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		itemID, err := scanUUID(a.ItemID)
		if err != nil {
			return errorToolResult("invalid_item_id", "item_id is not a valid UUID"), nil
		}

		if err := deps.Menus.DeleteMenuItem(ctx, itemID, tenantID); err != nil {
			return errorToolResult("menu_delete_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"item_id": uuidString(itemID),
			"deleted": true,
		}), nil
	}
}
