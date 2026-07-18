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

// PermissionsChecker provides RBAC permission resolution for menu item filtering.
type PermissionsChecker interface {
	HasPermission(ctx context.Context, userID string, permissionNode string) bool
}

// Service provides business logic for navigating menus and managing items.
type Service struct {
	store *Store
	db    *db.DB
}

func NewService(database *db.DB) *Service {
	return &Service{
		store: &Store{read: gen.New(database.Reader), write: gen.New(database.Writer)},
		db:    database,
	}
}

// ── Menu CRUD ─────────────────────────────────────────────────────────

type ListMenusResponse struct {
	Items      []gen.Menu `json:"items"`
	Page       int32      `json:"page"`
	TotalPages int32      `json:"total_pages"`
}

func (s *Service) ListMenus(ctx context.Context, tenantID pgtype.UUID, q pgtype.Text, limit, offset int32) (*ListMenusResponse, error) {
	menus, err := s.store.listMenus(ctx, tenantID, q, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing menus: %w", err)
	}
	total, err := s.store.countMenus(ctx, tenantID, q)
	if err != nil {
		return nil, fmt.Errorf("counting menus: %w", err)
	}
	totalPages := total / limit
	if total%limit > 0 {
		totalPages++
	}
	return &ListMenusResponse{Items: menus, Page: offset / limit, TotalPages: totalPages}, nil
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

func (s *Service) GetMenuBySlug(ctx context.Context, tenantID pgtype.UUID, slug string, userPermissions map[string]bool, isAuthenticated bool) (*MenuResponse, error) {
	menu, err := s.store.getMenuBySlug(ctx, tenantID, slug)
	if err != nil {
		return nil, fmt.Errorf("getting menu by slug: %w", err)
	}
	items, err := s.store.listMenuItems(ctx, tenantID, menu.ID)
	if err != nil {
		return nil, fmt.Errorf("listing menu items: %w", err)
	}
	jsonItems := genMenuItemsToJSON(items)
	filteredItems := filterMenuItems(jsonItems, userPermissions, isAuthenticated)
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

func (s *Service) CreateMenu(ctx context.Context, tenantID pgtype.UUID, slug, name, placement string) (gen.Menu, error) {
	if !ValidPlacement(placement) {
		return gen.Menu{}, ErrInvalidPlacement
	}
	return s.store.createMenu(ctx, gen.CreateMenuParams{
		TenantID: tenantID, Slug: slug, Name: name, Placement: placement,
	})
}

func (s *Service) UpdateMenu(ctx context.Context, id, tenantID pgtype.UUID, name, placement, status string) (gen.Menu, error) {
	if !ValidPlacement(placement) {
		return gen.Menu{}, ErrInvalidPlacement
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
	MenuID         string  `json:"menu_id"`
	ParentID       *string `json:"parent_id"`
	Label          string  `json:"label"`
	URL            string  `json:"url"`
	Icon           *string `json:"icon"`
	SortOrder      int     `json:"sort_order"`
	PermissionNode *string `json:"permission_node"`
	Visibility     string  `json:"visibility"`
	Target         string  `json:"target"`
	IsAdSlot       bool    `json:"is_ad_slot"`
	AdSlotConfig   json.RawMessage `json:"ad_slot_config"`
}

type UpdateMenuItemInput struct {
	Label          string  `json:"label"`
	URL            string  `json:"url"`
	Icon           *string `json:"icon"`
	ParentID       *string `json:"parent_id"`
	SortOrder      int     `json:"sort_order"`
	PermissionNode *string `json:"permission_node"`
	Visibility     string  `json:"visibility"`
	Target         string  `json:"target"`
	IsAdSlot       bool    `json:"is_ad_slot"`
	AdSlotConfig   json.RawMessage `json:"ad_slot_config"`
	Status         string  `json:"status"`
}

func (s *Service) CreateMenuItem(ctx context.Context, tenantID pgtype.UUID, input CreateMenuItemInput) (gen.MenuItem, error) {
	if !ValidVisibility(input.Visibility) {
		return gen.MenuItem{}, ErrInvalidVisibility
	}
	var menuID pgtype.UUID
	if err := menuID.Scan(input.MenuID); err != nil {
		return gen.MenuItem{}, fmt.Errorf("invalid menu_id: %w", err)
	}
	var parentID pgtype.UUID
	if input.ParentID != nil {
		if err := parentID.Scan(*input.ParentID); err != nil {
			return gen.MenuItem{}, fmt.Errorf("invalid parent_id: %w", err)
		}
	}
	var adConfig []byte
	if len(input.AdSlotConfig) > 0 {
		adConfig = input.AdSlotConfig
	}
	return s.store.createMenuItem(ctx, gen.CreateMenuItemParams{
		TenantID:       tenantID,
		MenuID:         menuID,
		ParentID:       parentID,
		Label:          input.Label,
		Url:            input.URL,
		Icon:           input.Icon,
		SortOrder:      int32(input.SortOrder),
		PermissionNode: input.PermissionNode,
		Visibility:     input.Visibility,
		Target:         input.Target,
		IsAdSlot:       input.IsAdSlot,
		AdSlotConfig:   adConfig,
	})
}

func (s *Service) UpdateMenuItem(ctx context.Context, id, tenantID pgtype.UUID, input UpdateMenuItemInput) (gen.MenuItem, error) {
	if !ValidVisibility(input.Visibility) {
		return gen.MenuItem{}, ErrInvalidVisibility
	}
	var parentID pgtype.UUID
	if input.ParentID != nil {
		if err := parentID.Scan(*input.ParentID); err != nil {
			return gen.MenuItem{}, fmt.Errorf("invalid parent_id: %w", err)
		}
	}
	var adConfig []byte
	if len(input.AdSlotConfig) > 0 {
		adConfig = input.AdSlotConfig
	}
	return s.store.updateMenuItem(ctx, gen.UpdateMenuItemParams{
		ID:             id,
		TenantID:       tenantID,
		Label:          input.Label,
		Url:            input.URL,
		Icon:           input.Icon,
		ParentID:       parentID,
		SortOrder:      int32(input.SortOrder),
		PermissionNode: input.PermissionNode,
		Visibility:     input.Visibility,
		Target:         input.Target,
		IsAdSlot:       input.IsAdSlot,
		AdSlotConfig:   adConfig,
		Status:         input.Status,
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

// ── Helpers ───────────────────────────────────────────────────────────

func genMenuItemsToJSON(rows []gen.MenuItem) []MenuItem {
	items := make([]MenuItem, len(rows))
	for i, row := range rows {
		items[i] = genMenuItemToJSON(row)
	}
	return items
}

// filterMenuItems removes items the user should not see based on auth state and permissions.
func filterMenuItems(items []MenuItem, userPermissions map[string]bool, isAuthenticated bool) []MenuItem {
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
		if item.PermissionNode != nil && *item.PermissionNode != "" {
			if !userPermissions[*item.PermissionNode] {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	return filtered
}
