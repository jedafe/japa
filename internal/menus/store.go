package menus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for menus and menu items.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
}

// NewStore is an exported function.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// ── Menus ─────────────────────────────────────────────────────────────

func (s *Store) listMenus(ctx context.Context, tenantID pgtype.UUID, q, placement, status, sort, sortDir string, limit, offset int32) ([]gen.ListMenusRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListMenus(ctx, gen.ListMenusParams{
		TenantID: tenantID, Q: q, Placement: placement, Status: status, SortDir: sortDir, Sort: sort,
		PageOffset: offset, PageLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing menus: %w", err)
	}

	return rows, nil
}

func (s *Store) countMenus(ctx context.Context, tenantID pgtype.UUID, q, placement, status string) (int32, error) {
	count, err := s.read.CountMenus(ctx, gen.CountMenusParams{
		TenantID: tenantID, Q: q, Placement: placement, Status: status,
	})
	if err != nil {
		return 0, fmt.Errorf("counting menus: %w", err)
	}

	return count, nil
}

func (s *Store) getMenuByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.Menu, error) {
	row, err := s.read.GetMenuByID(ctx, gen.GetMenuByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.Menu{}, ErrMenuNotFound
		}

		return gen.Menu{}, fmt.Errorf("getting menu: %w", err)
	}

	return row, nil
}

func (s *Store) getMenuBySlug(ctx context.Context, tenantID pgtype.UUID, slug string) (gen.Menu, error) {
	row, err := s.read.GetMenuBySlugForTenant(ctx, gen.GetMenuBySlugForTenantParams{TenantID: tenantID, Slug: slug})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.Menu{}, ErrMenuNotFound
		}

		return gen.Menu{}, fmt.Errorf("getting menu by slug: %w", err)
	}

	return row, nil
}

func (s *Store) createMenu(ctx context.Context, p gen.CreateMenuParams) (gen.Menu, error) {
	row, err := s.write.CreateMenu(ctx, p)
	if err != nil {
		// Map unique violation on (tenant_id, slug) to ErrDuplicateSlug so
		// the handler returns 409 already_exists instead of 500 (Rule 5).
		// Ref: ADR-054; matches the pattern in internal/sidebars/store.go.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return gen.Menu{}, ErrDuplicateSlug
		}

		return gen.Menu{}, fmt.Errorf("creating menu: %w", err)
	}

	return row, nil
}

func (s *Store) updateMenu(ctx context.Context, p gen.UpdateMenuParams) (gen.Menu, error) {
	row, err := s.write.UpdateMenu(ctx, p)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.Menu{}, ErrMenuNotFound
		}

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

func (s *Store) listMenuItems(ctx context.Context, tenantID, menuID pgtype.UUID) ([]gen.ListMenuItemsRow, error) {
	rows, err := s.read.ListMenuItems(ctx, gen.ListMenuItemsParams{TenantID: tenantID, MenuID: menuID})
	if err != nil {
		return nil, fmt.Errorf("listing menu items: %w", err)
	}

	return rows, nil
}

func (s *Store) getMenuItem(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetMenuItemRow, error) {
	row, err := s.read.GetMenuItem(ctx, gen.GetMenuItemParams{ID: id, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.GetMenuItemRow{}, ErrMenuItemNotFound
		}

		return gen.GetMenuItemRow{}, fmt.Errorf("getting menu item: %w", err)
	}

	return row, nil
}

func (s *Store) createMenuItem(ctx context.Context, p gen.CreateMenuItemParams) (gen.CreateMenuItemRow, error) {
	row, err := s.write.CreateMenuItem(ctx, p)
	if err != nil {
		return gen.CreateMenuItemRow{}, fmt.Errorf("creating menu item: %w", err)
	}

	return row, nil
}

func (s *Store) updateMenuItem(ctx context.Context, p gen.UpdateMenuItemParams) (gen.UpdateMenuItemRow, error) {
	row, err := s.write.UpdateMenuItem(ctx, p)
	if err != nil {
		return gen.UpdateMenuItemRow{}, fmt.Errorf("updating menu item: %w", err)
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

// ── User group resolution ──────────────────────────────────────────────

func (s *Store) getUserGroupIDs(ctx context.Context, tenantID, userID pgtype.UUID) ([]string, error) {
	rows, err := s.read.GetUserGroupIDs(ctx, gen.GetUserGroupIDsParams{
		TenantID: tenantID, UserID: userID,
	})
	if err != nil {
		return nil, fmt.Errorf("getting user group ids: %w", err)
	}

	return rows, nil
}

// ── JSON serialization helpers ────────────────────────────────────────

// MenuItem is the JSON-safe representation of a menu item with children.
type MenuItem struct {
	ID               string         `json:"id"`
	TenantID         string         `json:"tenant_id"`
	MenuID           string         `json:"menu_id"`
	ParentID         *string        `json:"parent_id,omitempty"`
	Label            string         `json:"label"`
	URL              string         `json:"url"`
	Icon             *string        `json:"icon,omitempty"`
	SortOrder        int            `json:"sort_order"`
	PermissionNode   *string        `json:"permission_node,omitempty"`
	RequiredGroupIDs []string       `json:"required_group_ids"`
	Visibility       string         `json:"visibility"`
	Target           string         `json:"target"`
	IsAdSlot         bool           `json:"is_ad_slot"`
	AdSlotConfig     map[string]any `json:"ad_slot_config,omitempty"`
	Status           string         `json:"status"`
	FeatureFlag      *string        `json:"feature_flag,omitempty"`
	Children         []MenuItem     `json:"children"`
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

// menuItemFields is a type constraint for any sqlc-generated row that has
// the menu item fields we need for JSON serialization.
type menuItemFields interface {
	gen.ListMenuItemsRow | gen.GetMenuItemRow | gen.CreateMenuItemRow | gen.UpdateMenuItemRow
}

// genMenuItemToJSON converts a sqlc-generated menu item row into a JSON-safe MenuItem.
func genMenuItemToJSON[T menuItemFields](row T) MenuItem {
	// All Row types have identical fields; convert via a helper that reads
	// the common fields by passing the row through a type switch.
	return menuItemRowToJSON(row)
}

// menuItemRowToJSON uses a type switch to extract fields from any Row type.
func menuItemRowToJSON(row any) MenuItem {
	type fields struct {
		ID               pgtype.UUID
		TenantID         pgtype.UUID
		MenuID           pgtype.UUID
		ParentID         pgtype.UUID
		Label            string
		Url              string
		Icon             *string
		SortOrder        int32
		PermissionNode   *string
		Visibility       string
		Target           string
		IsAdSlot         bool
		AdSlotConfig     []byte
		Status           string
		RequiredGroupIds []byte
		FeatureFlag      *string
	}

	f := fields{}
	switch r := row.(type) {
	case gen.ListMenuItemsRow:
		f = fields{
			r.ID, r.TenantID, r.MenuID, r.ParentID, r.Label, r.Url, r.Icon,
			r.SortOrder, r.PermissionNode, r.Visibility, r.Target, r.IsAdSlot,
			r.AdSlotConfig, r.Status, r.RequiredGroupIds, r.FeatureFlag,
		}
	case gen.GetMenuItemRow:
		f = fields{
			r.ID, r.TenantID, r.MenuID, r.ParentID, r.Label, r.Url, r.Icon,
			r.SortOrder, r.PermissionNode, r.Visibility, r.Target, r.IsAdSlot,
			r.AdSlotConfig, r.Status, r.RequiredGroupIds, r.FeatureFlag,
		}
	case gen.CreateMenuItemRow:
		f = fields{
			r.ID, r.TenantID, r.MenuID, r.ParentID, r.Label, r.Url, r.Icon,
			r.SortOrder, r.PermissionNode, r.Visibility, r.Target, r.IsAdSlot,
			r.AdSlotConfig, r.Status, r.RequiredGroupIds, r.FeatureFlag,
		}
	case gen.UpdateMenuItemRow:
		f = fields{
			r.ID, r.TenantID, r.MenuID, r.ParentID, r.Label, r.Url, r.Icon,
			r.SortOrder, r.PermissionNode, r.Visibility, r.Target, r.IsAdSlot,
			r.AdSlotConfig, r.Status, r.RequiredGroupIds, r.FeatureFlag,
		}
	default:
		return MenuItem{Children: []MenuItem{}}
	}

	item := MenuItem{
		ID:               f.ID.String(),
		TenantID:         f.TenantID.String(),
		MenuID:           f.MenuID.String(),
		Label:            f.Label,
		URL:              f.Url,
		SortOrder:        int(f.SortOrder),
		PermissionNode:   f.PermissionNode,
		RequiredGroupIDs: []string{},
		Visibility:       f.Visibility,
		Target:           f.Target,
		IsAdSlot:         f.IsAdSlot,
		Status:           f.Status,
		FeatureFlag:      f.FeatureFlag,
		Children:         []MenuItem{},
	}

	if f.ParentID.Valid {
		s := f.ParentID.String()
		item.ParentID = &s
	}

	if f.Icon != nil {
		item.Icon = f.Icon
	}

	if len(f.AdSlotConfig) > 0 && string(f.AdSlotConfig) != "null" {
		var cfg map[string]any
		if err := json.Unmarshal(f.AdSlotConfig, &cfg); err == nil {
			item.AdSlotConfig = cfg
		}
	}

	if len(f.RequiredGroupIds) > 0 && string(f.RequiredGroupIds) != "null" {
		var ids []string
		if err := json.Unmarshal(f.RequiredGroupIds, &ids); err == nil {
			item.RequiredGroupIDs = ids
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
