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

// List returns a paginated list of widget configurations for a platform.
func (s *Store) List(ctx context.Context, q string, page, limit int32) ([]Widget, int64, error) {
	offset := (page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	rows, err := s.reader.Query(ctx, `
		SELECT id, name, widget_type,
		       COALESCE(config, '{}'::jsonb),
		       cache_ttl_secs, is_active, created_at, updated_at
		FROM widget_configurations
		WHERE ($1 = '' OR name ILIKE '%' || $1 || '%' OR widget_type ILIKE '%' || $1 || '%')
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, q, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing widgets: %w", err)
	}
	defer rows.Close()

	widgets := make([]Widget, 0)

	for rows.Next() {
		var w Widget
		if err := rows.Scan(
			&w.ID, &w.Name, &w.WidgetType,
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
		WHERE ($1 = '' OR name ILIKE '%' || $1 || '%' OR widget_type ILIKE '%' || $1 || '%')
	`, q).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting widgets: %w", err)
	}

	return widgets, total, nil
}

// Get returns a single widget by ID, scoped to the platform.
func (s *Store) Get(ctx context.Context, id pgtype.UUID) (Widget, error) {
	var w Widget

	err := s.reader.QueryRow(ctx, `
		SELECT id, name, widget_type,
		       COALESCE(config, '{}'::jsonb),
		       cache_ttl_secs, is_active, created_at, updated_at
		FROM widget_configurations
		WHERE id = $1
	`, id).Scan(
		&w.ID, &w.Name, &w.WidgetType,
		&w.Config, &w.CacheTTLSecs, &w.IsActive, &w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		return Widget{}, fmt.Errorf("getting widget: %w", err)
	}

	return w, nil
}

// GetPublic returns a single active widget by ID (no platform scope — public
// render endpoint resolves platform from the widget row itself).
func (s *Store) GetPublic(ctx context.Context, id pgtype.UUID) (Widget, error) {
	var w Widget

	err := s.reader.QueryRow(ctx, `
		SELECT id, name, widget_type,
		       COALESCE(config, '{}'::jsonb),
		       cache_ttl_secs, is_active, created_at, updated_at
		FROM widget_configurations
		WHERE id = $1 AND is_active = true
	`, id).Scan(
		&w.ID, &w.Name, &w.WidgetType,
		&w.Config, &w.CacheTTLSecs, &w.IsActive, &w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		return Widget{}, fmt.Errorf("getting public widget: %w", err)
	}

	return w, nil
}

// Create inserts a new widget configuration.
func (s *Store) Create(ctx context.Context, input CreateWidgetInput) (Widget, error) {
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
		INSERT INTO widget_configurations (name, widget_type, config, cache_ttl_secs, is_active) VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, widget_type,
		          COALESCE(config, '{}'::jsonb),
		          cache_ttl_secs, is_active, created_at, updated_at
	`, input.Name, input.WidgetType, config, ttl, active).Scan(
		&w.ID, &w.Name, &w.WidgetType,
		&w.Config, &w.CacheTTLSecs, &w.IsActive, &w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		return Widget{}, fmt.Errorf("creating widget: %w", err)
	}

	return w, nil
}

// Update modifies an existing widget configuration. Nil fields are preserved.
func (s *Store) Update(ctx context.Context, id pgtype.UUID, input UpdateWidgetInput) (Widget, error) {
	var configArg any
	if input.Config != nil {
		configArg = *input.Config
	}

	var w Widget

	err := s.writer.QueryRow(ctx, `
		UPDATE widget_configurations
		SET name = COALESCE($2, name),
		    widget_type = COALESCE($3, widget_type),
		    config = COALESCE($4::jsonb, config),
		    cache_ttl_secs = COALESCE($5, cache_ttl_secs),
		    is_active = COALESCE($6, is_active),
		    updated_at = now()
		WHERE id = $1
		RETURNING id, name, widget_type,
		          COALESCE(config, '{}'::jsonb),
		          cache_ttl_secs, is_active, created_at, updated_at
	`, id, input.Name, input.WidgetType, configArg, input.CacheTTLSecs, input.IsActive).Scan(
		&w.ID, &w.Name, &w.WidgetType,
		&w.Config, &w.CacheTTLSecs, &w.IsActive, &w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		return Widget{}, fmt.Errorf("updating widget: %w", err)
	}

	return w, nil
}

// Delete removes a widget configuration.
func (s *Store) Delete(ctx context.Context, id pgtype.UUID) error {
	tag, err := s.writer.Exec(ctx, `DELETE FROM widget_configurations WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting widget: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return errpkg.ErrNotFound
	}

	return nil
}
