package widgets

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/db"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Store handles widget_configurations database operations.
type Store struct {
	reader *pgxpool.Pool
	writer *pgxpool.Pool
}

// NewStore creates a Store from the database pool.
func NewStore(database *db.DB) *Store {
	return &Store{
		reader: database.Reader,
		writer: database.Writer,
	}
}

// List returns a paginated list of widget configurations for a tenant.
func (s *Store) List(ctx context.Context, tenantID pgtype.UUID, q string, page, limit int32) ([]Widget, int64, error) {
	offset := (page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	rows, err := s.reader.Query(ctx, `
		SELECT id, tenant_id, name, widget_type,
		       COALESCE(config, '{}'::jsonb),
		       cache_ttl_secs, is_active, created_at, updated_at
		FROM widget_configurations
		WHERE tenant_id = $1
		  AND ($2 = '' OR name ILIKE '%' || $2 || '%' OR widget_type ILIKE '%' || $2 || '%')
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`, tenantID, q, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing widgets: %w", err)
	}
	defer rows.Close()

	widgets := make([]Widget, 0)

	for rows.Next() {
		var w Widget
		if err := rows.Scan(
			&w.ID, &w.TenantID, &w.Name, &w.WidgetType,
			&w.Config, &w.CacheTTLSecs, &w.IsActive, &w.CreatedAt, &w.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning widget: %w", err)
		}

		widgets = append(widgets, w)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var total int64

	err = s.reader.QueryRow(ctx, `
		SELECT COUNT(*) FROM widget_configurations
		WHERE tenant_id = $1
		  AND ($2 = '' OR name ILIKE '%' || $2 || '%' OR widget_type ILIKE '%' || $2 || '%')
	`, tenantID, q).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting widgets: %w", err)
	}

	return widgets, total, nil
}

// Get returns a single widget by ID, scoped to the tenant.
func (s *Store) Get(ctx context.Context, id, tenantID pgtype.UUID) (Widget, error) {
	var w Widget

	err := s.reader.QueryRow(ctx, `
		SELECT id, tenant_id, name, widget_type,
		       COALESCE(config, '{}'::jsonb),
		       cache_ttl_secs, is_active, created_at, updated_at
		FROM widget_configurations
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID).Scan(
		&w.ID, &w.TenantID, &w.Name, &w.WidgetType,
		&w.Config, &w.CacheTTLSecs, &w.IsActive, &w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		return Widget{}, fmt.Errorf("getting widget: %w", err)
	}

	return w, nil
}

// GetPublic returns a single active widget by ID (no tenant scope — public
// render endpoint resolves tenant from the widget row itself).
func (s *Store) GetPublic(ctx context.Context, id pgtype.UUID) (Widget, error) {
	var w Widget

	err := s.reader.QueryRow(ctx, `
		SELECT id, tenant_id, name, widget_type,
		       COALESCE(config, '{}'::jsonb),
		       cache_ttl_secs, is_active, created_at, updated_at
		FROM widget_configurations
		WHERE id = $1 AND is_active = true
	`, id).Scan(
		&w.ID, &w.TenantID, &w.Name, &w.WidgetType,
		&w.Config, &w.CacheTTLSecs, &w.IsActive, &w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		return Widget{}, fmt.Errorf("getting public widget: %w", err)
	}

	return w, nil
}

// Create inserts a new widget configuration.
func (s *Store) Create(ctx context.Context, tenantID pgtype.UUID, input CreateWidgetInput) (Widget, error) {
	config := input.Config
	if len(config) == 0 {
		config = json.RawMessage(`{}`)
	}

	ttl := int32(300)
	if input.CacheTTLSecs != nil && *input.CacheTTLSecs > 0 {
		ttl = *input.CacheTTLSecs
	}

	active := true
	if input.IsActive != nil {
		active = *input.IsActive
	}

	var w Widget

	err := s.writer.QueryRow(ctx, `
		INSERT INTO widget_configurations (tenant_id, name, widget_type, config, cache_ttl_secs, is_active)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, name, widget_type,
		          COALESCE(config, '{}'::jsonb),
		          cache_ttl_secs, is_active, created_at, updated_at
	`, tenantID, input.Name, input.WidgetType, config, ttl, active).Scan(
		&w.ID, &w.TenantID, &w.Name, &w.WidgetType,
		&w.Config, &w.CacheTTLSecs, &w.IsActive, &w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		return Widget{}, fmt.Errorf("creating widget: %w", err)
	}

	return w, nil
}

// Update modifies an existing widget configuration. Nil fields are preserved.
func (s *Store) Update(ctx context.Context, id, tenantID pgtype.UUID, input UpdateWidgetInput) (Widget, error) {
	var configArg any
	if input.Config != nil {
		configArg = *input.Config
	}

	var w Widget

	err := s.writer.QueryRow(ctx, `
		UPDATE widget_configurations
		SET name = COALESCE($3, name),
		    widget_type = COALESCE($4, widget_type),
		    config = COALESCE($5::jsonb, config),
		    cache_ttl_secs = COALESCE($6, cache_ttl_secs),
		    is_active = COALESCE($7, is_active),
		    updated_at = now()
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, name, widget_type,
		          COALESCE(config, '{}'::jsonb),
		          cache_ttl_secs, is_active, created_at, updated_at
	`, id, tenantID, input.Name, input.WidgetType, configArg, input.CacheTTLSecs, input.IsActive).Scan(
		&w.ID, &w.TenantID, &w.Name, &w.WidgetType,
		&w.Config, &w.CacheTTLSecs, &w.IsActive, &w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		return Widget{}, fmt.Errorf("updating widget: %w", err)
	}

	return w, nil
}

// Delete removes a widget configuration.
func (s *Store) Delete(ctx context.Context, id, tenantID pgtype.UUID) error {
	tag, err := s.writer.Exec(ctx, `DELETE FROM widget_configurations WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		return fmt.Errorf("deleting widget: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return errpkg.ErrNotFound
	}

	return nil
}
