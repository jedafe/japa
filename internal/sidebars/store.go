package sidebars

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for sidebars and sidebar widgets.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
}

func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// ── Sidebars ──────────────────────────────────────────────────────────

func (s *Store) listSidebars(ctx context.Context, tenantID pgtype.UUID, app, position, q string, limit, offset int32) ([]gen.Sidebar, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}
	rows, err := s.read.ListSidebars(ctx, gen.ListSidebarsParams{
		TenantID: tenantID, App: app, Position: position, Q: q,
		PageOffset: offset, PageLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing sidebars: %w", err)
	}
	return rows, nil
}

func (s *Store) countSidebars(ctx context.Context, tenantID pgtype.UUID, app, position, q string) (int32, error) {
	count, err := s.read.CountSidebars(ctx, gen.CountSidebarsParams{
		TenantID: tenantID, App: app, Position: position, Q: q,
	})
	if err != nil {
		return 0, fmt.Errorf("counting sidebars: %w", err)
	}
	return count, nil
}

func (s *Store) getSidebarByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.Sidebar, error) {
	row, err := s.read.GetSidebarByID(ctx, gen.GetSidebarByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.Sidebar{}, fmt.Errorf("getting sidebar: %w", err)
	}
	return row, nil
}

func (s *Store) createSidebar(ctx context.Context, p gen.CreateSidebarParams) (gen.Sidebar, error) {
	row, err := s.write.CreateSidebar(ctx, p)
	if err != nil {
		return gen.Sidebar{}, fmt.Errorf("creating sidebar: %w", err)
	}
	return row, nil
}

func (s *Store) updateSidebar(ctx context.Context, p gen.UpdateSidebarParams) (gen.Sidebar, error) {
	row, err := s.write.UpdateSidebar(ctx, p)
	if err != nil {
		return gen.Sidebar{}, fmt.Errorf("updating sidebar: %w", err)
	}
	return row, nil
}

func (s *Store) deleteSidebar(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteSidebar(ctx, gen.DeleteSidebarParams{ID: id, TenantID: tenantID}); err != nil {
		return fmt.Errorf("deleting sidebar: %w", err)
	}
	return nil
}

// ── Sidebar Widgets ───────────────────────────────────────────────────

func (s *Store) listSidebarWidgets(ctx context.Context, tenantID, sidebarID pgtype.UUID) ([]gen.SidebarWidget, error) {
	rows, err := s.read.ListSidebarWidgets(ctx, gen.ListSidebarWidgetsParams{
		TenantID: tenantID, SidebarID: sidebarID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing sidebar widgets: %w", err)
	}
	return rows, nil
}

func (s *Store) getSidebarWidget(ctx context.Context, id, tenantID pgtype.UUID) (gen.SidebarWidget, error) {
	row, err := s.read.GetSidebarWidget(ctx, gen.GetSidebarWidgetParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.SidebarWidget{}, fmt.Errorf("getting sidebar widget: %w", err)
	}
	return row, nil
}

func (s *Store) createSidebarWidget(ctx context.Context, p gen.CreateSidebarWidgetParams) (gen.SidebarWidget, error) {
	row, err := s.write.CreateSidebarWidget(ctx, p)
	if err != nil {
		return gen.SidebarWidget{}, fmt.Errorf("creating sidebar widget: %w", err)
	}
	return row, nil
}

func (s *Store) updateSidebarWidget(ctx context.Context, p gen.UpdateSidebarWidgetParams) (gen.SidebarWidget, error) {
	row, err := s.write.UpdateSidebarWidget(ctx, p)
	if err != nil {
		return gen.SidebarWidget{}, fmt.Errorf("updating sidebar widget: %w", err)
	}
	return row, nil
}

func (s *Store) deleteSidebarWidget(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteSidebarWidget(ctx, gen.DeleteSidebarWidgetParams{ID: id, TenantID: tenantID}); err != nil {
		return fmt.Errorf("deleting sidebar widget: %w", err)
	}
	return nil
}

func (s *Store) bulkUpdateSidebarWidgetSortOrder(ctx context.Context, tenantID pgtype.UUID, ids []pgtype.UUID, sortOrders []int32) error {
	if err := s.write.BulkUpdateSidebarWidgetSortOrder(ctx, gen.BulkUpdateSidebarWidgetSortOrderParams{
		TenantID: tenantID, Ids: ids, SortOrders: sortOrders,
	}); err != nil {
		return fmt.Errorf("bulk updating sidebar widget sort order: %w", err)
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

// ── Public resolution ──────────────────────────────────────────────────

func (s *Store) listActiveSidebarsForResolve(ctx context.Context, tenantID pgtype.UUID, app, position string) ([]gen.Sidebar, error) {
	rows, err := s.read.ListActiveSidebarsForResolve(ctx, gen.ListActiveSidebarsForResolveParams{
		TenantID: tenantID, App: app, Position: position,
	})
	if err != nil {
		return nil, fmt.Errorf("listing active sidebars for resolve: %w", err)
	}
	return rows, nil
}

func (s *Store) listActiveSidebarWidgets(ctx context.Context, tenantID, sidebarID pgtype.UUID) ([]gen.SidebarWidget, error) {
	rows, err := s.read.ListActiveSidebarWidgets(ctx, gen.ListActiveSidebarWidgetsParams{
		TenantID: tenantID, SidebarID: sidebarID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing active sidebar widgets: %w", err)
	}
	return rows, nil
}

// ── JSON serialization helpers ────────────────────────────────────────

// SidebarWidgetJSON is the JSON-safe representation of a sidebar widget.
type SidebarWidgetJSON struct {
	ID               string         `json:"id"`
	TenantID         string         `json:"tenant_id"`
	SidebarID        string         `json:"sidebar_id"`
	WidgetType       string         `json:"widget_type"`
	Label            string         `json:"label"`
	Props            map[string]any `json:"props"`
	SortOrder        int            `json:"sort_order"`
	Visibility       string         `json:"visibility"`
	RequiredGroupIDs []string       `json:"required_group_ids"`
	PermissionNode   *string        `json:"permission_node,omitempty"`
	Status           string         `json:"status"`
}

// SidebarJSON is the JSON-safe representation of a sidebar.
type SidebarJSON struct {
	ID               string                `json:"id"`
	TenantID         string                `json:"tenant_id"`
	App              string                `json:"app"`
	Position         string                `json:"position"`
	Slug             string                `json:"slug"`
	Name             string                `json:"name"`
	PagePattern      string                `json:"page_pattern"`
	Visibility       string                `json:"visibility"`
	RequiredGroupIDs []string              `json:"required_group_ids"`
	Status           string                `json:"status"`
	Widgets          []SidebarWidgetJSON   `json:"widgets"`
}

// genSidebarToJSON converts a sqlc-generated Sidebar row into a JSON-safe SidebarJSON.
func genSidebarToJSON(row gen.Sidebar) SidebarJSON {
	s := SidebarJSON{
		ID:               row.ID.String(),
		TenantID:         row.TenantID.String(),
		App:              row.App,
		Position:         row.Position,
		Slug:             row.Slug,
		Name:             row.Name,
		PagePattern:      row.PagePattern,
		Visibility:       row.Visibility,
		RequiredGroupIDs: []string{},
		Status:           row.Status,
		Widgets:          []SidebarWidgetJSON{},
	}
	if len(row.RequiredGroupIds) > 0 && string(row.RequiredGroupIds) != "null" {
		var ids []string
		if err := json.Unmarshal(row.RequiredGroupIds, &ids); err == nil {
			s.RequiredGroupIDs = ids
		}
	}
	return s
}

// genSidebarWidgetToJSON converts a sqlc-generated SidebarWidget row into a JSON-safe SidebarWidgetJSON.
func genSidebarWidgetToJSON(row gen.SidebarWidget) SidebarWidgetJSON {
	w := SidebarWidgetJSON{
		ID:               row.ID.String(),
		TenantID:         row.TenantID.String(),
		SidebarID:        row.SidebarID.String(),
		WidgetType:       row.WidgetType,
		Label:            row.Label,
		Props:            map[string]any{},
		SortOrder:        int(row.SortOrder),
		Visibility:       row.Visibility,
		RequiredGroupIDs: []string{},
		PermissionNode:   row.PermissionNode,
		Status:           row.Status,
	}
	if len(row.Props) > 0 && string(row.Props) != "null" {
		var props map[string]any
		if err := json.Unmarshal(row.Props, &props); err == nil {
			w.Props = props
		}
	}
	if len(row.RequiredGroupIds) > 0 && string(row.RequiredGroupIds) != "null" {
		var ids []string
		if err := json.Unmarshal(row.RequiredGroupIds, &ids); err == nil {
			w.RequiredGroupIDs = ids
		}
	}
	return w
}

func genSidebarWidgetsToJSON(rows []gen.SidebarWidget) []SidebarWidgetJSON {
	widgets := make([]SidebarWidgetJSON, len(rows))
	for i, row := range rows {
		widgets[i] = genSidebarWidgetToJSON(row)
	}
	return widgets
}
