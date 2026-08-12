package pagebuilder

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/db"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Store provides pagebuilder data access with reader/writer pool separation.
type Store struct {
	reader *pgxpool.Pool
	writer *pgxpool.Pool
}

// NewStore creates a Store backed by the given database pools.
func NewStore(database *db.DB) *Store {
	return &Store{
		reader: database.Reader,
		writer: database.Writer,
	}
}

// ---------------------------------------------------------------------------
// Blocks CRUD
// ---------------------------------------------------------------------------

// ListBlocks returns a paginated list of blocks for a tenant, ordered by name.
// page is 1-based; limit is the page size. Returns the items and the total row
// count (before pagination) so the handler can compute total_pages.
func (s *Store) ListBlocks(ctx context.Context, tenantID pgtype.UUID, q string, page, limit int32) ([]Block, int64, error) {
	offset := (page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	rows, err := s.reader.Query(ctx, `
		SELECT id, tenant_id, name, slug, category, component,
		       COALESCE(default_config, '{}'::jsonb),
		       COALESCE(config_schema, '{}'::jsonb),
		       preview_image, is_system, is_enabled, created_at, updated_at
		FROM pagebuilder_blocks
		WHERE tenant_id = $1
		  AND ($2 = '' OR name ILIKE '%' || $2 || '%' OR slug ILIKE '%' || $2 || '%' OR category ILIKE '%' || $2 || '%')
		ORDER BY name
		LIMIT $3 OFFSET $4
	`, tenantID, q, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing blocks: %w", err)
	}
	defer rows.Close()

	var blocks []Block
	blocks = make([]Block, 0)

	for rows.Next() {
		var b Block
		if err := rows.Scan(
			&b.ID, &b.TenantID, &b.Name, &b.Slug, &b.Category, &b.Component,
			&b.DefaultConfig, &b.ConfigSchema,
			&b.PreviewImage, &b.IsSystem, &b.IsEnabled, &b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning block: %w", err)
		}

		blocks = append(blocks, b)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var total int64
	err = s.reader.QueryRow(ctx, `
		SELECT COUNT(*) FROM pagebuilder_blocks
		WHERE tenant_id = $1
		  AND ($2 = '' OR name ILIKE '%' || $2 || '%' OR slug ILIKE '%' || $2 || '%' OR category ILIKE '%' || $2 || '%')
	`, tenantID, q).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting blocks: %w", err)
	}

	return blocks, total, nil
}

// GetBlock returns a single block by ID, scoped to the tenant.
func (s *Store) GetBlock(ctx context.Context, id, tenantID pgtype.UUID) (Block, error) {
	var b Block

	err := s.reader.QueryRow(ctx, `
		SELECT id, tenant_id, name, slug, category, component,
		       COALESCE(default_config, '{}'::jsonb),
		       COALESCE(config_schema, '{}'::jsonb),
		       preview_image, is_system, is_enabled, created_at, updated_at
		FROM pagebuilder_blocks
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID).Scan(
		&b.ID, &b.TenantID, &b.Name, &b.Slug, &b.Category, &b.Component,
		&b.DefaultConfig, &b.ConfigSchema,
		&b.PreviewImage, &b.IsSystem, &b.IsEnabled, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return Block{}, fmt.Errorf("getting block: %w", err)
	}

	return b, nil
}

// CreateBlock inserts a new block row.
func (s *Store) CreateBlock(ctx context.Context, tenantID pgtype.UUID, input CreateBlockInput) (Block, error) {
	defaultConfig := input.DefaultConfig
	if defaultConfig == nil {
		defaultConfig = json.RawMessage(`{}`)
	}

	configSchema := input.ConfigSchema
	if configSchema == nil {
		configSchema = json.RawMessage(`{}`)
	}

	var b Block

	err := s.writer.QueryRow(
		ctx, `
		INSERT INTO pagebuilder_blocks (tenant_id, name, slug, category, component, default_config, config_schema, preview_image, is_enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, tenant_id, name, slug, category, component,
		          COALESCE(default_config, '{}'::jsonb),
		          COALESCE(config_schema, '{}'::jsonb),
		          preview_image, is_system, is_enabled, created_at, updated_at
	`, tenantID, input.Name, input.Slug, input.Category, input.Component,
		defaultConfig, configSchema, input.PreviewImage, input.IsEnabled,
	).Scan(
		&b.ID, &b.TenantID, &b.Name, &b.Slug, &b.Category, &b.Component,
		&b.DefaultConfig, &b.ConfigSchema,
		&b.PreviewImage, &b.IsSystem, &b.IsEnabled, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return Block{}, fmt.Errorf("creating block: %w", err)
	}

	return b, nil
}

// UpdateBlock updates an existing block row.
func (s *Store) UpdateBlock(ctx context.Context, id, tenantID pgtype.UUID, input UpdateBlockInput) (Block, error) {
	defaultConfig := input.DefaultConfig
	if defaultConfig == nil {
		defaultConfig = json.RawMessage(`{}`)
	}

	configSchema := input.ConfigSchema
	if configSchema == nil {
		configSchema = json.RawMessage(`{}`)
	}

	var b Block

	err := s.writer.QueryRow(
		ctx, `
		UPDATE pagebuilder_blocks
		SET name = $3, slug = $4, category = $5, component = $6,
		    default_config = $7, config_schema = $8, preview_image = $9,
		    is_enabled = $10, updated_at = now()
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, name, slug, category, component,
		          COALESCE(default_config, '{}'::jsonb),
		          COALESCE(config_schema, '{}'::jsonb),
		          preview_image, is_system, is_enabled, created_at, updated_at
	`, id, tenantID, input.Name, input.Slug, input.Category, input.Component,
		defaultConfig, configSchema, input.PreviewImage, input.IsEnabled,
	).Scan(
		&b.ID, &b.TenantID, &b.Name, &b.Slug, &b.Category, &b.Component,
		&b.DefaultConfig, &b.ConfigSchema,
		&b.PreviewImage, &b.IsSystem, &b.IsEnabled, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return Block{}, fmt.Errorf("updating block: %w", err)
	}

	return b, nil
}

// DeleteBlock removes a block row. System blocks cannot be deleted.
func (s *Store) DeleteBlock(ctx context.Context, id, tenantID pgtype.UUID) error {
	tag, err := s.writer.Exec(ctx, `
		DELETE FROM pagebuilder_blocks
		WHERE id = $1 AND tenant_id = $2 AND is_system = false
	`, id, tenantID)
	if err != nil {
		return fmt.Errorf("deleting block: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: block not found or is a system block", errpkg.ErrForbidden)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Templates CRUD
// ---------------------------------------------------------------------------

// ListTemplates returns a paginated list of templates for a tenant, optionally
// filtered by q and category. page is 1-based; limit is the page size.
func (s *Store) ListTemplates(ctx context.Context, tenantID pgtype.UUID, q, category string, page, limit int32) ([]Template, int64, error) {
	offset := (page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	rows, err := s.reader.Query(ctx, `
		SELECT id, tenant_id, name, slug, description, page_type,
		       COALESCE(blocks, '[]'::jsonb), is_system, created_at, updated_at
		FROM pagebuilder_templates
		WHERE tenant_id = $1
		  AND ($2 = '' OR name ILIKE '%' || $2 || '%' OR slug ILIKE '%' || $2 || '%' OR description ILIKE '%' || $2 || '%')
		  AND ($3 = '' OR page_type = $3)
		ORDER BY name
		LIMIT $4 OFFSET $5
	`, tenantID, q, category, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing templates: %w", err)
	}
	defer rows.Close()

	var templates []Template
	templates = make([]Template, 0)

	for rows.Next() {
		var t Template
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.Name, &t.Slug, &t.Description, &t.PageType,
			&t.Blocks, &t.IsSystem, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning template: %w", err)
		}

		templates = append(templates, t)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var total int64
	err = s.reader.QueryRow(ctx, `
		SELECT COUNT(*) FROM pagebuilder_templates
		WHERE tenant_id = $1
		  AND ($2 = '' OR name ILIKE '%' || $2 || '%' OR slug ILIKE '%' || $2 || '%' OR description ILIKE '%' || $2 || '%')
		  AND ($3 = '' OR page_type = $3)
	`, tenantID, q, category).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting templates: %w", err)
	}

	return templates, total, nil
}

// GetTemplate returns a single template by ID, scoped to the tenant.
func (s *Store) GetTemplate(ctx context.Context, id, tenantID pgtype.UUID) (Template, error) {
	var t Template

	err := s.reader.QueryRow(ctx, `
		SELECT id, tenant_id, name, slug, description, page_type,
		       COALESCE(blocks, '[]'::jsonb), is_system, created_at, updated_at
		FROM pagebuilder_templates
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID).Scan(
		&t.ID, &t.TenantID, &t.Name, &t.Slug, &t.Description, &t.PageType,
		&t.Blocks, &t.IsSystem, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return Template{}, fmt.Errorf("getting template: %w", err)
	}

	return t, nil
}

// CreateTemplate inserts a new template row.
func (s *Store) CreateTemplate(ctx context.Context, tenantID pgtype.UUID, input CreateTemplateInput) (Template, error) {
	blocks := input.Blocks
	if blocks == nil {
		blocks = json.RawMessage(`[]`)
	}

	var t Template

	err := s.writer.QueryRow(
		ctx, `
		INSERT INTO pagebuilder_templates (tenant_id, name, slug, description, page_type, blocks)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, name, slug, description, page_type,
		          COALESCE(blocks, '[]'::jsonb), is_system, created_at, updated_at
	`, tenantID, input.Name, input.Slug, input.Description, input.PageType, blocks,
	).Scan(
		&t.ID, &t.TenantID, &t.Name, &t.Slug, &t.Description, &t.PageType,
		&t.Blocks, &t.IsSystem, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return Template{}, fmt.Errorf("creating template: %w", err)
	}

	return t, nil
}

// UpdateTemplate updates an existing template row.
func (s *Store) UpdateTemplate(ctx context.Context, id, tenantID pgtype.UUID, input UpdateTemplateInput) (Template, error) {
	blocks := input.Blocks
	if blocks == nil {
		blocks = json.RawMessage(`[]`)
	}

	var t Template

	err := s.writer.QueryRow(
		ctx, `
		UPDATE pagebuilder_templates
		SET name = $3, slug = $4, description = $5, page_type = $6,
		    blocks = $7, updated_at = now()
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, name, slug, description, page_type,
		          COALESCE(blocks, '[]'::jsonb), is_system, created_at, updated_at
	`, id, tenantID, input.Name, input.Slug, input.Description, input.PageType, blocks,
	).Scan(
		&t.ID, &t.TenantID, &t.Name, &t.Slug, &t.Description, &t.PageType,
		&t.Blocks, &t.IsSystem, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return Template{}, fmt.Errorf("updating template: %w", err)
	}

	return t, nil
}

// DeleteTemplate removes a template row. System templates cannot be deleted.
func (s *Store) DeleteTemplate(ctx context.Context, id, tenantID pgtype.UUID) error {
	tag, err := s.writer.Exec(ctx, `
		DELETE FROM pagebuilder_templates
		WHERE id = $1 AND tenant_id = $2 AND is_system = false
	`, id, tenantID)
	if err != nil {
		return fmt.Errorf("deleting template: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: template not found or is a system template", errpkg.ErrForbidden)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Themes CRUD
// ---------------------------------------------------------------------------

// ListThemes returns a paginated list of themes for a tenant, ordered by name.
func (s *Store) ListThemes(ctx context.Context, tenantID pgtype.UUID, q string, page, limit int32) ([]Theme, int64, error) {
	offset := (page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	rows, err := s.reader.Query(ctx, `
		SELECT id, tenant_id, name, slug, description,
		       COALESCE(colors, '{}'::jsonb),
		       COALESCE(typography, '{}'::jsonb),
		       COALESCE(spacing, '{}'::jsonb),
		       is_default, created_at, updated_at
		FROM pagebuilder_themes
		WHERE tenant_id = $1
		  AND ($2 = '' OR name ILIKE '%' || $2 || '%' OR slug ILIKE '%' || $2 || '%' OR description ILIKE '%' || $2 || '%')
		ORDER BY name
		LIMIT $3 OFFSET $4
	`, tenantID, q, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing themes: %w", err)
	}
	defer rows.Close()

	var themes []Theme
	themes = make([]Theme, 0)

	for rows.Next() {
		var t Theme
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.Name, &t.Slug, &t.Description,
			&t.Colors, &t.Typography, &t.Spacing,
			&t.IsDefault, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning theme: %w", err)
		}

		themes = append(themes, t)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var total int64
	err = s.reader.QueryRow(ctx, `
		SELECT COUNT(*) FROM pagebuilder_themes
		WHERE tenant_id = $1
		  AND ($2 = '' OR name ILIKE '%' || $2 || '%' OR slug ILIKE '%' || $2 || '%' OR description ILIKE '%' || $2 || '%')
	`, tenantID, q).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting themes: %w", err)
	}

	return themes, total, nil
}

// GetTheme returns a single theme by ID, scoped to the tenant.
func (s *Store) GetTheme(ctx context.Context, id, tenantID pgtype.UUID) (Theme, error) {
	var t Theme

	err := s.reader.QueryRow(ctx, `
		SELECT id, tenant_id, name, slug, description,
		       COALESCE(colors, '{}'::jsonb),
		       COALESCE(typography, '{}'::jsonb),
		       COALESCE(spacing, '{}'::jsonb),
		       is_default, created_at, updated_at
		FROM pagebuilder_themes
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID).Scan(
		&t.ID, &t.TenantID, &t.Name, &t.Slug, &t.Description,
		&t.Colors, &t.Typography, &t.Spacing,
		&t.IsDefault, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return Theme{}, fmt.Errorf("getting theme: %w", err)
	}

	return t, nil
}

// CreateTheme inserts a new theme row.
func (s *Store) CreateTheme(ctx context.Context, tenantID pgtype.UUID, input CreateThemeInput) (Theme, error) {
	colors := input.Colors
	if colors == nil {
		colors = json.RawMessage(`{}`)
	}

	typography := input.Typography
	if typography == nil {
		typography = json.RawMessage(`{}`)
	}

	spacing := input.Spacing
	if spacing == nil {
		spacing = json.RawMessage(`{}`)
	}

	var t Theme

	err := s.writer.QueryRow(
		ctx, `
		INSERT INTO pagebuilder_themes (tenant_id, name, slug, description, colors, typography, spacing, is_default)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, tenant_id, name, slug, description,
		          COALESCE(colors, '{}'::jsonb),
		          COALESCE(typography, '{}'::jsonb),
		          COALESCE(spacing, '{}'::jsonb),
		          is_default, created_at, updated_at
	`, tenantID, input.Name, input.Slug, input.Description, colors, typography, spacing, input.IsDefault,
	).Scan(
		&t.ID, &t.TenantID, &t.Name, &t.Slug, &t.Description,
		&t.Colors, &t.Typography, &t.Spacing,
		&t.IsDefault, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return Theme{}, fmt.Errorf("creating theme: %w", err)
	}

	return t, nil
}

// UpdateTheme updates an existing theme row.
func (s *Store) UpdateTheme(ctx context.Context, id, tenantID pgtype.UUID, input UpdateThemeInput) (Theme, error) {
	colors := input.Colors
	if colors == nil {
		colors = json.RawMessage(`{}`)
	}

	typography := input.Typography
	if typography == nil {
		typography = json.RawMessage(`{}`)
	}

	spacing := input.Spacing
	if spacing == nil {
		spacing = json.RawMessage(`{}`)
	}

	var t Theme

	err := s.writer.QueryRow(
		ctx, `
		UPDATE pagebuilder_themes
		SET name = $3, slug = $4, description = $5, colors = $6,
		    typography = $7, spacing = $8, is_default = $9, updated_at = now()
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, name, slug, description,
		          COALESCE(colors, '{}'::jsonb),
		          COALESCE(typography, '{}'::jsonb),
		          COALESCE(spacing, '{}'::jsonb),
		          is_default, created_at, updated_at
	`, id, tenantID, input.Name, input.Slug, input.Description, colors, typography, spacing, input.IsDefault,
	).Scan(
		&t.ID, &t.TenantID, &t.Name, &t.Slug, &t.Description,
		&t.Colors, &t.Typography, &t.Spacing,
		&t.IsDefault, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return Theme{}, fmt.Errorf("updating theme: %w", err)
	}

	return t, nil
}

// DeleteTheme removes a theme row.
func (s *Store) DeleteTheme(ctx context.Context, id, tenantID pgtype.UUID) error {
	tag, err := s.writer.Exec(ctx, `
		DELETE FROM pagebuilder_themes
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID)
	if err != nil {
		return fmt.Errorf("deleting theme: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: theme not found", errpkg.ErrNotFound)
	}

	return nil
}

// SetDefaultTheme clears the default flag for all themes in the tenant,
// then sets the given theme as default.
func (s *Store) SetDefaultTheme(ctx context.Context, id, tenantID pgtype.UUID) error {
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		UPDATE pagebuilder_themes SET is_default = false WHERE tenant_id = $1
	`, tenantID); err != nil {
		return fmt.Errorf("clearing default themes: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE pagebuilder_themes SET is_default = true, updated_at = now()
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID)
	if err != nil {
		return fmt.Errorf("setting default theme: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: theme not found", errpkg.ErrNotFound)
	}

	return tx.Commit(ctx)
}

// ---------------------------------------------------------------------------
// Pages CRUD
// ---------------------------------------------------------------------------

// PageListFilter narrows a ListPages query. Empty fields are ignored.
type PageListFilter struct {
	Q        string // free-text match on title/slug/description
	PageType string // exact match on page_type (e.g. "landing")
	Slug     string // exact match on slug
	Status   string // exact match on status
}

// ListPages returns a paginated list of pages for a tenant, ordered by
// created_at DESC, optionally narrowed by the supplied filter.
func (s *Store) ListPages(ctx context.Context, tenantID pgtype.UUID, f PageListFilter, page, limit int32) ([]Page, int64, error) {
	offset := (page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	rows, err := s.reader.Query(ctx, `
		SELECT id, tenant_id, title, slug, description, template_id, theme_id,
		       COALESCE(blocks, '[]'::jsonb),
		       COALESCE(meta, '{}'::jsonb),
		       status, page_type, published_at, created_by, created_at, updated_at
		FROM pages
		WHERE tenant_id = $1
		  AND ($2 = '' OR title ILIKE '%' || $2 || '%' OR slug ILIKE '%' || $2 || '%' OR description ILIKE '%' || $2 || '%')
		  AND ($3 = '' OR page_type = $3)
		  AND ($4 = '' OR slug = $4)
		  AND ($5 = '' OR status = $5)
		ORDER BY created_at DESC
		LIMIT $6 OFFSET $7
	`, tenantID, f.Q, f.PageType, f.Slug, f.Status, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing pages: %w", err)
	}
	defer rows.Close()

	var pages []Page
	pages = make([]Page, 0)

	for rows.Next() {
		var p Page
		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.Title, &p.Slug, &p.Description,
			&p.TemplateID, &p.ThemeID, &p.Blocks, &p.Meta,
			&p.Status, &p.PageType, &p.PublishedAt, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning page: %w", err)
		}

		pages = append(pages, p)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var total int64
	err = s.reader.QueryRow(ctx, `
		SELECT COUNT(*) FROM pages
		WHERE tenant_id = $1
		  AND ($2 = '' OR title ILIKE '%' || $2 || '%' OR slug ILIKE '%' || $2 || '%' OR description ILIKE '%' || $2 || '%')
		  AND ($3 = '' OR page_type = $3)
		  AND ($4 = '' OR slug = $4)
		  AND ($5 = '' OR status = $5)
	`, tenantID, f.Q, f.PageType, f.Slug, f.Status).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting pages: %w", err)
	}

	return pages, total, nil
}

// ListPublishedPages returns published pages for public consumption.
func (s *Store) ListPublishedPages(ctx context.Context, tenantID pgtype.UUID) ([]Page, error) {
	rows, err := s.reader.Query(ctx, `
		SELECT id, tenant_id, title, slug, description, template_id, theme_id,
		       COALESCE(blocks, '[]'::jsonb),
		       COALESCE(meta, '{}'::jsonb),
		       status, page_type, published_at, created_by, created_at, updated_at
		FROM pages
		WHERE tenant_id = $1 AND status = 'published' AND page_type IN ('page', 'landing')
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing published pages: %w", err)
	}
	defer rows.Close()

	var pages []Page
	pages = make([]Page, 0)

	for rows.Next() {
		var p Page
		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.Title, &p.Slug, &p.Description,
			&p.TemplateID, &p.ThemeID, &p.Blocks, &p.Meta,
			&p.Status, &p.PageType, &p.PublishedAt, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning page: %w", err)
		}

		pages = append(pages, p)
	}

	return pages, rows.Err()
}

// GetPage returns a single page by ID, scoped to the tenant.
func (s *Store) GetPage(ctx context.Context, id, tenantID pgtype.UUID) (Page, error) {
	var p Page

	err := s.reader.QueryRow(ctx, `
		SELECT id, tenant_id, title, slug, description, template_id, theme_id,
		       COALESCE(blocks, '[]'::jsonb),
		       COALESCE(meta, '{}'::jsonb),
		       status, page_type, published_at, created_by, created_at, updated_at
		FROM pages
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID).Scan(
		&p.ID, &p.TenantID, &p.Title, &p.Slug, &p.Description,
		&p.TemplateID, &p.ThemeID, &p.Blocks, &p.Meta,
		&p.Status, &p.PageType, &p.PublishedAt, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return Page{}, fmt.Errorf("getting page: %w", err)
	}

	return p, nil
}

// GetPageBySlug returns a published page by slug for public consumption.
func (s *Store) GetPageBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (Page, error) {
	var p Page

	err := s.reader.QueryRow(ctx, `
		SELECT id, tenant_id, title, slug, description, template_id, theme_id,
		       COALESCE(blocks, '[]'::jsonb),
		       COALESCE(meta, '{}'::jsonb),
		       status, page_type, published_at, created_by, created_at, updated_at
		FROM pages
		WHERE slug = $1 AND tenant_id = $2 AND status = 'published' AND page_type IN ('page', 'landing')
	`, slug, tenantID).Scan(
		&p.ID, &p.TenantID, &p.Title, &p.Slug, &p.Description,
		&p.TemplateID, &p.ThemeID, &p.Blocks, &p.Meta,
		&p.Status, &p.PageType, &p.PublishedAt, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return Page{}, fmt.Errorf("getting page by slug: %w", err)
	}

	return p, nil
}

// SlugExists reports whether any page (any status) already uses slug for this
// tenant. Used by DuplicatePage to derive a unique slug for the copy.
func (s *Store) SlugExists(ctx context.Context, slug string, tenantID pgtype.UUID) (bool, error) {
	var exists bool
	err := s.reader.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM pages WHERE slug = $1 AND tenant_id = $2)
	`, slug, tenantID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking slug existence: %w", err)
	}
	return exists, nil
}

// CreatePage inserts a new page row.
func (s *Store) CreatePage(ctx context.Context, tenantID, createdBy pgtype.UUID, input CreatePageInput) (Page, error) {
	blocks := input.Blocks
	if blocks == nil {
		blocks = json.RawMessage(`[]`)
	}

	meta := input.Meta
	if meta == nil {
		meta = json.RawMessage(`{}`)
	}

	status := input.Status
	if status == "" {
		status = PageStatusDraft
	}

	pageType := input.PageType
	if pageType == "" {
		pageType = "page"
	}

	var p Page

	err := s.writer.QueryRow(
		ctx, `
		INSERT INTO pages (tenant_id, title, slug, description, template_id, theme_id, blocks, meta, status, page_type, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, tenant_id, title, slug, description, template_id, theme_id,
		          COALESCE(blocks, '[]'::jsonb),
		          COALESCE(meta, '{}'::jsonb),
		          status, page_type, published_at, created_by, created_at, updated_at
	`, tenantID, input.Title, input.Slug, input.Description, input.TemplateID, input.ThemeID,
		blocks, meta, status, pageType, createdBy,
	).Scan(
		&p.ID, &p.TenantID, &p.Title, &p.Slug, &p.Description,
		&p.TemplateID, &p.ThemeID, &p.Blocks, &p.Meta,
		&p.Status, &p.PageType, &p.PublishedAt, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return Page{}, fmt.Errorf("creating page: %w", err)
	}

	return p, nil
}

// UpdatePage updates an existing page row.
//
// Omitted fields are preserved: when the client sends only title+blocks (the
// common admin-editor save), description/template_id/theme_id/meta are kept
// intact via COALESCE. A nil json.RawMessage is passed as SQL NULL so COALESCE
// falls back to the existing column value; a non-nil RawMessage (including `[]`
// or `{}`) overwrites. Ref: §16; §1.3 data-loss fix.
func (s *Store) UpdatePage(ctx context.Context, id, tenantID pgtype.UUID, input UpdatePageInput) (Page, error) {
	var blocks any
	if input.Blocks != nil {
		blocks = input.Blocks
	}

	var meta any
	if input.Meta != nil {
		meta = input.Meta
	}

	var p Page

	err := s.writer.QueryRow(
		ctx, `
		UPDATE pages
		SET title = $3,
		    slug = COALESCE(NULLIF($4, ''), slug),
		    description = COALESCE($5, description),
		    template_id = COALESCE($6, template_id),
		    theme_id = COALESCE($7, theme_id),
		    blocks = COALESCE($8::jsonb, blocks),
		    meta = COALESCE($9::jsonb, meta),
		    status = COALESCE(NULLIF($10, ''), status),
		    page_type = COALESCE(NULLIF($11, ''), page_type),
		    updated_at = now()
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, title, slug, description, template_id, theme_id,
		          COALESCE(blocks, '[]'::jsonb),
		          COALESCE(meta, '{}'::jsonb),
		          status, page_type, published_at, created_by, created_at, updated_at
	`, id, tenantID, input.Title, input.Slug, input.Description, input.TemplateID,
		input.ThemeID, blocks, meta, input.Status, input.PageType,
	).Scan(
		&p.ID, &p.TenantID, &p.Title, &p.Slug, &p.Description,
		&p.TemplateID, &p.ThemeID, &p.Blocks, &p.Meta,
		&p.Status, &p.PageType, &p.PublishedAt, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return Page{}, fmt.Errorf("updating page: %w", err)
	}

	return p, nil
}

// DeletePage removes a page row.
func (s *Store) DeletePage(ctx context.Context, id, tenantID pgtype.UUID) error {
	tag, err := s.writer.Exec(ctx, `
		DELETE FROM pages WHERE id = $1 AND tenant_id = $2
	`, id, tenantID)
	if err != nil {
		return fmt.Errorf("deleting page: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: page not found", errpkg.ErrNotFound)
	}

	return nil
}

// PublishPage sets the page status to 'published' and stamps published_at.
func (s *Store) PublishPage(ctx context.Context, id, tenantID pgtype.UUID) (Page, error) {
	var p Page

	err := s.writer.QueryRow(ctx, `
		UPDATE pages
		SET status = 'published', published_at = now(), updated_at = now()
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, title, slug, description, template_id, theme_id,
		          COALESCE(blocks, '[]'::jsonb),
		          COALESCE(meta, '{}'::jsonb),
		          status, page_type, published_at, created_by, created_at, updated_at
	`, id, tenantID).Scan(
		&p.ID, &p.TenantID, &p.Title, &p.Slug, &p.Description,
		&p.TemplateID, &p.ThemeID, &p.Blocks, &p.Meta,
		&p.Status, &p.PageType, &p.PublishedAt, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return Page{}, fmt.Errorf("publishing page: %w", err)
	}

	return p, nil
}

// ArchivePage sets the page status to 'archived'.
func (s *Store) ArchivePage(ctx context.Context, id, tenantID pgtype.UUID) (Page, error) {
	var p Page

	err := s.writer.QueryRow(ctx, `
		UPDATE pages
		SET status = 'archived', published_at = NULL, updated_at = now()
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, title, slug, description, template_id, theme_id,
		          COALESCE(blocks, '[]'::jsonb),
		          COALESCE(meta, '{}'::jsonb),
		          status, page_type, published_at, created_by, created_at, updated_at
	`, id, tenantID).Scan(
		&p.ID, &p.TenantID, &p.Title, &p.Slug, &p.Description,
		&p.TemplateID, &p.ThemeID, &p.Blocks, &p.Meta,
		&p.Status, &p.PageType, &p.PublishedAt, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return Page{}, fmt.Errorf("archiving page: %w", err)
	}

	return p, nil
}
