package menus

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// PermissionResolver resolves a user's full permission set in one call.
// rbac.RBAC satisfies this via GetUserContext (L1 in-memory → L2 Valkey → DB),
// so the public menu resolver pays one cached lookup per request instead of
// one HasPermission call per menu item. Ref: ADR-054 (Menu item visibility
// rules — "the public endpoint calls rbac.GetUserContext for authenticated
// users. This is already cached").
type PermissionResolver interface {
	UserPermissions(ctx context.Context, userID, tenantID pgtype.UUID) (map[string]bool, error)
}

// Service provides business logic for navigating menus and managing items.
type Service struct {
	store    *Store
	db       *db.DB
	perms    PermissionResolver
}

// NewService constructs a menus Service without RBAC permission resolution
// (menu item permission_node filtering is disabled). Prefer NewServiceWithPerms
// for the public resolver path so authenticated users are gated by
// permission_node per ADR-054.
func NewService(database *db.DB) *Service {
	return &Service{
		store: &Store{read: gen.New(database.Reader), write: gen.New(database.Writer)},
		db:    database,
	}
}

// NewServiceWithPerms constructs a menus Service wired to the RBAC resolver so
// the public GET /menus/{slug} endpoint can prune items whose
// permission_node is not held by the requesting user (ADR-054).
func NewServiceWithPerms(database *db.DB, perms PermissionResolver) *Service {
	s := NewService(database)
	s.perms = perms
	return s
}

// ── Menu CRUD ─────────────────────────────────────────────────────────

type ListMenusResponse struct {
	Items      []gen.ListMenusRow `json:"items"`
	Page       int32              `json:"page"`
	TotalPages int32              `json:"total_pages"`
	Total      int32              `json:"total"`
}

func (s *Service) ListMenus(ctx context.Context, tenantID pgtype.UUID, q, placement, status, sort, sortDir string, limit, offset int32) (*ListMenusResponse, error) {
	menus, err := s.store.listMenus(ctx, tenantID, q, placement, status, sort, sortDir, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing menus: %w", err)
	}
	total, err := s.store.countMenus(ctx, tenantID, q, placement, status)
	if err != nil {
		return nil, fmt.Errorf("counting menus: %w", err)
	}
	totalPages := total / limit
	if total%limit > 0 {
		totalPages++
	}
	return &ListMenusResponse{Items: menus, Page: offset / limit, TotalPages: totalPages, Total: total}, nil
}

func (s *Service) GetMenu(ctx context.Context, id, tenantID pgtype.UUID) (*MenuResponse, error) {
	menu, err := s.store.getMenuByID(ctx, id, tenantID)
	if err != nil {
		return nil, fmt.Errorf("getting menu: %w", err)
	}
	items, err := s.store.listMenuItems(ctx, tenantID, menu.ID)
	if err != nil {
		return nil, fmt.Errorf("listing menu items: %w", err)
	}
	treeItems := buildMenuTree(genMenuItemsToJSON(items))
	return &MenuResponse{
		ID:        menu.ID.String(),
		TenantID:  menu.TenantID.String(),
		Slug:      menu.Slug,
		Name:      menu.Name,
		Placement: menu.Placement,
		Status:    menu.Status,
		Items:     treeItems,
	}, nil
}

func (s *Service) GetMenuBySlug(ctx context.Context, tenantID pgtype.UUID, slug string, userGroupIDs map[string]bool, isAuthenticated bool, userID pgtype.UUID) (*MenuResponse, error) {
	menu, err := s.store.getMenuBySlug(ctx, tenantID, slug)
	if err != nil {
		return nil, fmt.Errorf("getting menu by slug: %w", err)
	}
	items, err := s.store.listMenuItems(ctx, tenantID, menu.ID)
	if err != nil {
		return nil, fmt.Errorf("listing menu items: %w", err)
	}
	jsonItems := genMenuItemsToJSON(items)

	// Resolve the authenticated user's permission set once per request so
	// filterMenuItems can prune items whose permission_node is not held.
	// ADR-054: "Authenticated users: see items with visibility IN
	// ('all','authenticated'), further filtered by permission_node — items
	// whose permission_node is non-null and not in the user's resolved
	// permissions are pruned." Guests skip this (no permission_node pruning).
	var userPerms map[string]bool
	if isAuthenticated && userID.Valid && s.perms != nil {
		perms, err := s.perms.UserPermissions(ctx, userID, tenantID)
		if err == nil {
			userPerms = perms
		}
		// On resolution failure we fail CLOSED: userPerms stays nil, and
		// filterMenuItems prunes every permission_node-gated item for the
		// authenticated user. Hiding a nav item on an infra failure is
		// preferable to leaking an admin-only item to a member. The RBAC
		// resolver is cached, so this path is hit only on an actual infra
		// failure. (ADR-054: items whose permission_node is non-null and
		// not in the user's resolved permissions are pruned.)
	}

	filteredItems := filterMenuItems(jsonItems, userGroupIDs, isAuthenticated, userPerms)
	treeItems := buildMenuTree(filteredItems)
	return &MenuResponse{
		ID:        menu.ID.String(),
		TenantID:  menu.TenantID.String(),
		Slug:      menu.Slug,
		Name:      menu.Name,
		Placement: menu.Placement,
		Status:    menu.Status,
		Items:     treeItems,
	}, nil
}

func (s *Service) CreateMenu(ctx context.Context, tenantID pgtype.UUID, slug, name, placement, status string) (gen.Menu, error) {
	if !ValidPlacement(placement) {
		return gen.Menu{}, ErrInvalidPlacement
	}
	if status == "" {
		status = string(StatusActive)
	}
	if !ValidStatus(status) {
		return gen.Menu{}, ErrInvalidStatus
	}
	return s.store.createMenu(ctx, gen.CreateMenuParams{
		TenantID: tenantID, Slug: slug, Name: name, Placement: placement, Status: status,
	})
}

func (s *Service) UpdateMenu(ctx context.Context, id, tenantID pgtype.UUID, name, placement, status string) (gen.Menu, error) {
	if !ValidPlacement(placement) {
		return gen.Menu{}, ErrInvalidPlacement
	}
	if !ValidStatus(status) {
		return gen.Menu{}, ErrInvalidStatus
	}
	return s.store.updateMenu(ctx, gen.UpdateMenuParams{
		ID: id, TenantID: tenantID, Name: name, Placement: placement, Status: status,
	})
}

func (s *Service) DeleteMenu(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.deleteMenu(ctx, id, tenantID)
}

// ── Menu Item CRUD ────────────────────────────────────────────────────

type CreateMenuItemInput struct {
	MenuID           string         `json:"menu_id"`
	ParentID         *string        `json:"parent_id"`
	Label            string         `json:"label"`
	URL              string         `json:"url"`
	Icon             *string        `json:"icon"`
	SortOrder        int            `json:"sort_order"`
	PermissionNode   *string        `json:"permission_node"`
	RequiredGroupIDs []string       `json:"required_group_ids"`
	Visibility       string         `json:"visibility"`
	Target           string         `json:"target"`
	IsAdSlot         bool           `json:"is_ad_slot"`
	AdSlotConfig     json.RawMessage `json:"ad_slot_config"`
	Status           string         `json:"status"`
}

type UpdateMenuItemInput struct {
	Label            string  `json:"label"`
	URL              string  `json:"url"`
	Icon             *string `json:"icon"`
	ParentID         *string `json:"parent_id"`
	SortOrder        int     `json:"sort_order"`
	PermissionNode   *string `json:"permission_node"`
	RequiredGroupIDs []string `json:"required_group_ids"`
	Visibility       string  `json:"visibility"`
	Target           string  `json:"target"`
	IsAdSlot         bool    `json:"is_ad_slot"`
	AdSlotConfig     json.RawMessage `json:"ad_slot_config"`
	Status           string  `json:"status"`
}

func (s *Service) CreateMenuItem(ctx context.Context, tenantID pgtype.UUID, input CreateMenuItemInput) (gen.CreateMenuItemRow, error) {
	if !ValidVisibility(input.Visibility) {
		return gen.CreateMenuItemRow{}, ErrInvalidVisibility
	}
	if !ValidTarget(input.Target) {
		return gen.CreateMenuItemRow{}, ErrInvalidTarget
	}
	status := input.Status
	if status == "" {
		status = string(StatusActive)
	}
	if !ValidStatus(status) {
		return gen.CreateMenuItemRow{}, ErrInvalidStatus
	}
	var menuID pgtype.UUID
	if err := menuID.Scan(input.MenuID); err != nil {
		return gen.CreateMenuItemRow{}, fmt.Errorf("invalid menu_id: %w", err)
	}
	var parentID pgtype.UUID
	if input.ParentID != nil {
		if err := parentID.Scan(*input.ParentID); err != nil {
			return gen.CreateMenuItemRow{}, fmt.Errorf("invalid parent_id: %w", err)
		}
	}
	var adConfig []byte
	if len(input.AdSlotConfig) > 0 {
		adConfig = input.AdSlotConfig
	}
	groupIDsJSON, err := json.Marshal(input.RequiredGroupIDs)
	if err != nil {
		return gen.CreateMenuItemRow{}, fmt.Errorf("encoding required_group_ids: %w", err)
	}
	if len(groupIDsJSON) == 0 {
		groupIDsJSON = []byte("[]")
	}
	return s.store.createMenuItem(ctx, gen.CreateMenuItemParams{
		TenantID:         tenantID,
		MenuID:           menuID,
		ParentID:         parentID,
		Label:            input.Label,
		Url:              input.URL,
		Icon:             input.Icon,
		SortOrder:        int32(input.SortOrder),
		PermissionNode:   input.PermissionNode,
		Visibility:       input.Visibility,
		Target:           input.Target,
		IsAdSlot:         input.IsAdSlot,
		AdSlotConfig:     adConfig,
		RequiredGroupIds: groupIDsJSON,
		Status:           status,
	})
}

func (s *Service) UpdateMenuItem(ctx context.Context, id, tenantID pgtype.UUID, input UpdateMenuItemInput) (gen.UpdateMenuItemRow, error) {
	if !ValidVisibility(input.Visibility) {
		return gen.UpdateMenuItemRow{}, ErrInvalidVisibility
	}
	if !ValidStatus(input.Status) {
		return gen.UpdateMenuItemRow{}, ErrInvalidStatus
	}
	if !ValidTarget(input.Target) {
		return gen.UpdateMenuItemRow{}, ErrInvalidTarget
	}
	var parentID pgtype.UUID
	if input.ParentID != nil {
		if err := parentID.Scan(*input.ParentID); err != nil {
			return gen.UpdateMenuItemRow{}, fmt.Errorf("invalid parent_id: %w", err)
		}
	}
	var adConfig []byte
	if len(input.AdSlotConfig) > 0 {
		adConfig = input.AdSlotConfig
	}
	groupIDsJSON, err := json.Marshal(input.RequiredGroupIDs)
	if err != nil {
		return gen.UpdateMenuItemRow{}, fmt.Errorf("encoding required_group_ids: %w", err)
	}
	if len(groupIDsJSON) == 0 {
		groupIDsJSON = []byte("[]")
	}
	return s.store.updateMenuItem(ctx, gen.UpdateMenuItemParams{
		ID:               id,
		TenantID:         tenantID,
		Label:            input.Label,
		Url:              input.URL,
		Icon:             input.Icon,
		ParentID:         parentID,
		SortOrder:        int32(input.SortOrder),
		PermissionNode:   input.PermissionNode,
		Visibility:       input.Visibility,
		Target:           input.Target,
		IsAdSlot:         input.IsAdSlot,
		AdSlotConfig:     adConfig,
		Status:           input.Status,
		RequiredGroupIds: groupIDsJSON,
	})
}

func (s *Service) DeleteMenuItem(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.deleteMenuItem(ctx, id, tenantID)
}

type ReorderInput struct {
	Items []ReorderItem `json:"items"`
}
type ReorderItem struct {
	ID        string `json:"id"`
	SortOrder int    `json:"sort_order"`
}

func (s *Service) ReorderMenuItems(ctx context.Context, tenantID pgtype.UUID, items []ReorderItem) error {
	if len(items) == 0 {
		return nil
	}
	// Single unnest-based UPDATE replaces the per-item loop: one round trip,
	// and the reorder is atomic instead of failing halfway.
	ids := make([]pgtype.UUID, len(items))
	sortOrders := make([]int32, len(items))
	for i, item := range items {
		if err := ids[i].Scan(item.ID); err != nil {
			return fmt.Errorf("invalid item id %s: %w", item.ID, err)
		}
		sortOrders[i] = int32(item.SortOrder)
	}
	if err := s.store.bulkUpdateMenuItemSortOrder(ctx, tenantID, ids, sortOrders); err != nil {
		slog.Error("reorder menu items failed", "tenant_id", tenantID.String(), "error", err)
		return err
	}
	return nil
}

// GetUserGroupIDs returns the set of user group IDs the user belongs to.
// Used for group-based menu item visibility filtering.
func (s *Service) GetUserGroupIDs(ctx context.Context, tenantID pgtype.UUID, userID string) ([]string, error) {
	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}
	return s.store.getUserGroupIDs(ctx, tenantID, uid)
}

// ── Helpers ───────────────────────────────────────────────────────────

func genMenuItemsToJSON(rows []gen.ListMenuItemsRow) []MenuItem {
	items := make([]MenuItem, len(rows))
	for i, row := range rows {
		items[i] = genMenuItemToJSON(row)
	}
	return items
}

// filterMenuItems removes items the user should not see based on auth state,
// required user group membership, and RBAC permission_node. An item with
// required_group_ids is shown only if the user belongs to at least one of the
// listed groups. Items with no required_group_ids are visible to everyone
// (subject to visibility rules). For authenticated users, an item with a
// non-nil permission_node is shown only if the user holds that permission.
// Guests skip permission_node pruning (ADR-054: guests need no permission
// check). Items with no permission_node are visible to everyone per visibility
// rules.
func filterMenuItems(items []MenuItem, userGroupIDs map[string]bool, isAuthenticated bool, userPerms map[string]bool) []MenuItem {
	var filtered []MenuItem
	for _, item := range items {
		if item.Status != "active" {
			continue
		}
		switch item.Visibility {
		case "guest":
			if isAuthenticated {
				continue
			}
		case "authenticated":
			if !isAuthenticated {
				continue
			}
		}
		// Group-based gating: if the item specifies required groups, the
		// user must be a member of at least one. An empty list means no
		// group restriction (visible to everyone per visibility rules).
		if len(item.RequiredGroupIDs) > 0 {
			matched := false
			for _, gid := range item.RequiredGroupIDs {
				if userGroupIDs[gid] {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		// Permission-node gating (ADR-054): an authenticated user sees an
		// item with a non-nil permission_node only if they hold that node.
		// A nil permission_node means no permission restriction. The
		// wildcard "*" (held by super_admin) is represented in userPerms by
		// the rbac.SystemWildcardPermission key, so a direct map lookup
		// covers both explicit grants and the wildcard.
		if isAuthenticated && item.PermissionNode != nil && *item.PermissionNode != "" {
			if userPerms == nil || !userPerms[*item.PermissionNode] {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	return filtered
}
