package menus

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for menus and menu items.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
}

func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// ── Menus ─────────────────────────────────────────────────────────────

func (s *Store) listMenus(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.Menu, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}
	rows, err := s.read.ListMenus(ctx, gen.ListMenusParams{
		TenantID: tenantID, PageOffset: offset, PageLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing menus: %w", err)
	}
	return rows, nil
}

func (s *Store) countMenus(ctx context.Context, tenantID pgtype.UUID) (int32, error) {
	count, err := s.read.CountMenus(ctx, tenantID)
	if err != nil {
		return 0, fmt.Errorf("counting menus: %w", err)
	}
	return count, nil
}

func (s *Store) getMenuByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.Menu, error) {
	row, err := s.read.GetMenuByID(ctx, gen.GetMenuByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.Menu{}, fmt.Errorf("getting menu: %w", err)
	}
	return row, nil
}

func (s *Store) getMenuBySlug(ctx context.Context, tenantID pgtype.UUID, slug string) (gen.Menu, error) {
	row, err := s.read.GetMenuBySlugForTenant(ctx, gen.GetMenuBySlugForTenantParams{TenantID: tenantID, Slug: slug})
	if err != nil {
		return gen.Menu{}, fmt.Errorf("getting menu by slug: %w", err)
	}
	return row, nil
}

func (s *Store) createMenu(ctx context.Context, p gen.CreateMenuParams) (gen.Menu, error) {
	row, err := s.write.CreateMenu(ctx, p)
	if err != nil {
		return gen.Menu{}, fmt.Errorf("creating menu: %w", err)
	}
	return row, nil
}

func (s *Store) updateMenu(ctx context.Context, p gen.UpdateMenuParams) (gen.Menu, error) {
	row, err := s.write.UpdateMenu(ctx, p)
	if err != nil {
		return gen.Menu{}, fmt.Errorf("updating menu: %w", err)
	}
	return row, nil
}

func (s *Store) deleteMenu(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteMenu(ctx, gen.DeleteMenuParams{ID: id, TenantID: tenantID}); err != nil {
		return fmt.Errorf("deleting menu: %w", err)
	}
	return nil
}

// ── Menu Items ────────────────────────────────────────────────────────

func (s *Store) listMenuItems(ctx context.Context, tenantID, menuID pgtype.UUID) ([]gen.MenuItem, error) {
	rows, err := s.read.ListMenuItems(ctx, gen.ListMenuItemsParams{TenantID: tenantID, MenuID: menuID})
	if err != nil {
		return nil, fmt.Errorf("listing menu items: %w", err)
	}
	return rows, nil
}

func (s *Store) getMenuItem(ctx context.Context, id, tenantID pgtype.UUID) (gen.MenuItem, error) {
	row, err := s.read.GetMenuItem(ctx, gen.GetMenuItemParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.MenuItem{}, fmt.Errorf("getting menu item: %w", err)
	}
	return row, nil
}

func (s *Store) createMenuItem(ctx context.Context, p gen.CreateMenuItemParams) (gen.MenuItem, error) {
	row, err := s.write.CreateMenuItem(ctx, p)
	if err != nil {
		return gen.MenuItem{}, fmt.Errorf("creating menu item: %w", err)
	}
	return row, nil
}

func (s *Store) updateMenuItem(ctx context.Context, p gen.UpdateMenuItemParams) (gen.MenuItem, error) {
	row, err := s.write.UpdateMenuItem(ctx, p)
	if err != nil {
		return gen.MenuItem{}, fmt.Errorf("updating menu item: %w", err)
	}
	return row, nil
}

func (s *Store) deleteMenuItem(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteMenuItem(ctx, gen.DeleteMenuItemParams{ID: id, TenantID: tenantID}); err != nil {
		return fmt.Errorf("deleting menu item: %w", err)
	}
	return nil
}

func (s *Store) bulkUpdateMenuItemSortOrder(ctx context.Context, tenantID pgtype.UUID, ids []pgtype.UUID, sortOrders []int32) error {
	if err := s.write.BulkUpdateMenuItemSortOrder(ctx, gen.BulkUpdateMenuItemSortOrderParams{
		TenantID: tenantID, Ids: ids, SortOrders: sortOrders,
	}); err != nil {
		return fmt.Errorf("bulk updating menu item sort order: %w", err)
	}
	return nil
}

// ── JSON serialization helpers ────────────────────────────────────────

// MenuItem is the JSON-safe representation of a menu item with children.
type MenuItem struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenant_id"`
	MenuID         string         `json:"menu_id"`
	ParentID       *string        `json:"parent_id,omitempty"`
	Label          string         `json:"label"`
	URL            string         `json:"url"`
	Icon           *string        `json:"icon,omitempty"`
	SortOrder      int            `json:"sort_order"`
	PermissionNode *string        `json:"permission_node,omitempty"`
	Visibility     string         `json:"visibility"`
	Target         string         `json:"target"`
	IsAdSlot       bool           `json:"is_ad_slot"`
	AdSlotConfig   map[string]any `json:"ad_slot_config,omitempty"`
	Status         string         `json:"status"`
	Children       []MenuItem     `json:"children"`
}

// MenuResponse is the JSON response for a menu with resolved items.
type MenuResponse struct {
	ID        string     `json:"id"`
	TenantID  string     `json:"tenant_id"`
	Slug      string     `json:"slug"`
	Name      string     `json:"name"`
	Placement string     `json:"placement"`
	Status    string     `json:"status"`
	Items     []MenuItem `json:"items"`
}

// genMenuItemToJSON converts a gen.MenuItem into a JSON-safe MenuItem.
func genMenuItemToJSON(row gen.MenuItem) MenuItem {
	item := MenuItem{
		ID:             row.ID.String(),
		TenantID:       row.TenantID.String(),
		MenuID:         row.MenuID.String(),
		Label:          row.Label,
		URL:            row.Url,
		SortOrder:      int(row.SortOrder),
		PermissionNode: row.PermissionNode,
		Visibility:     string(row.Visibility),
		Target:         string(row.Target),
		IsAdSlot:       row.IsAdSlot,
		Status:         string(row.Status),
		Children:       []MenuItem{},
	}

	if row.ParentID.Valid {
		s := row.ParentID.String()
		item.ParentID = &s
	}
	if row.Icon != nil {
		item.Icon = row.Icon
	}
	if len(row.AdSlotConfig) > 0 && string(row.AdSlotConfig) != "null" {
		var cfg map[string]any
		if err := json.Unmarshal(row.AdSlotConfig, &cfg); err == nil {
			item.AdSlotConfig = cfg
		}
	}

	return item
}

// buildMenuTree converts a flat list of menu items into a nested tree.
func buildMenuTree(items []MenuItem) []MenuItem {
	byParent := make(map[string][]MenuItem)
	var roots []MenuItem

	for _, item := range items {
		if item.ParentID == nil {
			roots = append(roots, item)
		} else {
			byParent[*item.ParentID] = append(byParent[*item.ParentID], item)
		}
	}

	for i := range roots {
		populateChildren(&roots[i], byParent)
	}

	return roots
}

func populateChildren(item *MenuItem, byParent map[string][]MenuItem) {
	children, ok := byParent[item.ID]
	if !ok {
		return
	}
	for _, child := range children {
		childCopy := child
		populateChildren(&childCopy, byParent)
		item.Children = append(item.Children, childCopy)
	}
}
