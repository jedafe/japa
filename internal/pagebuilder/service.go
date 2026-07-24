package pagebuilder

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Service provides pagebuilder business logic with reader/writer pool separation.
type Service struct {
	store *Store
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{store: NewStore(database)}
}

// validationError wraps a human-readable validation message with the
// ErrRequired sentinel so handleServiceError can map it to a 400 response
// (Rule 5 / §1.1) instead of surfacing a generic 500.
func validationError(msg string) error {
	return fmt.Errorf("%w: %s", errpkg.ErrRequired, msg)
}

// ---------------------------------------------------------------------------
// Block operations
// ---------------------------------------------------------------------------

// ListBlocks returns all blocks for a tenant, optionally filtered by q.
func (s *Service) ListBlocks(ctx context.Context, tenantID pgtype.UUID, q string) ([]Block, error) {
	return s.store.ListBlocks(ctx, tenantID, q)
}

// GetBlock returns a single block by ID, scoped to the tenant.
func (s *Service) GetBlock(ctx context.Context, id, tenantID pgtype.UUID) (Block, error) {
	return s.store.GetBlock(ctx, id, tenantID)
}

// CreateBlock creates a new block with input validation.
func (s *Service) CreateBlock(ctx context.Context, tenantID pgtype.UUID, input CreateBlockInput) (Block, error) {
	if input.Name == "" {
		return Block{}, validationError("name is required")
	}

	if input.Slug == "" {
		return Block{}, validationError("slug is required")
	}

	if input.Component == "" {
		return Block{}, validationError("component is required")
	}

	if input.Category == "" {
		input.Category = "general"
	}

	return s.store.CreateBlock(ctx, tenantID, input)
}

// UpdateBlock updates an existing block with input validation.
func (s *Service) UpdateBlock(ctx context.Context, id, tenantID pgtype.UUID, input UpdateBlockInput) (Block, error) {
	if input.Name == "" {
		return Block{}, validationError("name is required")
	}

	if input.Component == "" {
		return Block{}, validationError("component is required")
	}

	return s.store.UpdateBlock(ctx, id, tenantID, input)
}

// DeleteBlock removes a block. System blocks cannot be deleted.
func (s *Service) DeleteBlock(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.DeleteBlock(ctx, id, tenantID)
}

// ---------------------------------------------------------------------------
// Template operations
// ---------------------------------------------------------------------------

// ListTemplates returns all templates for a tenant, optionally filtered by q and category.
func (s *Service) ListTemplates(ctx context.Context, tenantID pgtype.UUID, q, category string) ([]Template, error) {
	return s.store.ListTemplates(ctx, tenantID, q, category)
}

// GetTemplate returns a single template by ID, scoped to the tenant.
func (s *Service) GetTemplate(ctx context.Context, id, tenantID pgtype.UUID) (Template, error) {
	return s.store.GetTemplate(ctx, id, tenantID)
}

// CreateTemplate creates a new template with input validation.
func (s *Service) CreateTemplate(ctx context.Context, tenantID pgtype.UUID, input CreateTemplateInput) (Template, error) {
	if input.Name == "" {
		return Template{}, validationError("name is required")
	}

	if input.Slug == "" {
		input.Slug = slugify(input.Name)
	}

	if input.PageType == "" {
		input.PageType = "custom"
	}

	return s.store.CreateTemplate(ctx, tenantID, input)
}

// UpdateTemplate updates an existing template with input validation.
func (s *Service) UpdateTemplate(ctx context.Context, id, tenantID pgtype.UUID, input UpdateTemplateInput) (Template, error) {
	if input.Name == "" {
		return Template{}, validationError("name is required")
	}

	if input.Slug == "" {
		input.Slug = slugify(input.Name)
	}

	return s.store.UpdateTemplate(ctx, id, tenantID, input)
}

// DeleteTemplate removes a template. System templates cannot be deleted.
func (s *Service) DeleteTemplate(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.DeleteTemplate(ctx, id, tenantID)
}

// ---------------------------------------------------------------------------
// Theme operations
// ---------------------------------------------------------------------------

// ListThemes returns all themes for a tenant, optionally filtered by q.
func (s *Service) ListThemes(ctx context.Context, tenantID pgtype.UUID, q string) ([]Theme, error) {
	return s.store.ListThemes(ctx, tenantID, q)
}

// GetTheme returns a single theme by ID, scoped to the tenant.
func (s *Service) GetTheme(ctx context.Context, id, tenantID pgtype.UUID) (Theme, error) {
	return s.store.GetTheme(ctx, id, tenantID)
}

// CreateTheme creates a new theme with input validation.
func (s *Service) CreateTheme(ctx context.Context, tenantID pgtype.UUID, input CreateThemeInput) (Theme, error) {
	if input.Name == "" {
		return Theme{}, validationError("name is required")
	}

	if input.Slug == "" {
		return Theme{}, validationError("slug is required")
	}

	return s.store.CreateTheme(ctx, tenantID, input)
}

// UpdateTheme updates an existing theme with input validation.
func (s *Service) UpdateTheme(ctx context.Context, id, tenantID pgtype.UUID, input UpdateThemeInput) (Theme, error) {
	if input.Name == "" {
		return Theme{}, validationError("name is required")
	}

	return s.store.UpdateTheme(ctx, id, tenantID, input)
}

// DeleteTheme removes a theme.
func (s *Service) DeleteTheme(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.DeleteTheme(ctx, id, tenantID)
}

// SetDefaultTheme sets a theme as the default for the tenant.
func (s *Service) SetDefaultTheme(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.SetDefaultTheme(ctx, id, tenantID)
}

// ---------------------------------------------------------------------------
// Page operations
// ---------------------------------------------------------------------------

// ListPages returns all pages for a tenant, optionally filtered by q.
func (s *Service) ListPages(ctx context.Context, tenantID pgtype.UUID, q string) ([]Page, error) {
	return s.store.ListPages(ctx, tenantID, q)
}

// GetPage returns a single page by ID, scoped to the tenant.
func (s *Service) GetPage(ctx context.Context, id, tenantID pgtype.UUID) (Page, error) {
	return s.store.GetPage(ctx, id, tenantID)
}

// GetPageBySlug returns a published page by slug for public consumption.
func (s *Service) GetPageBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (Page, error) {
	return s.store.GetPageBySlug(ctx, slug, tenantID)
}

// CreatePage creates a new page with input validation. When a template_id is
// provided and no blocks are supplied, the template's block array is copied
// into the new page (Section 16: templates are "starting points for page
// creation").
func (s *Service) CreatePage(ctx context.Context, tenantID, createdBy pgtype.UUID, input CreatePageInput) (Page, error) {
	if input.Title == "" {
		return Page{}, validationError("title is required")
	}

	if input.Slug == "" {
		input.Slug = slugify(input.Title)
	}

	if input.Status == "" {
		input.Status = PageStatusDraft
	}

	// If a template is selected and no blocks were explicitly provided,
	// copy the template's blocks into the new page (§16 template wiring).
	if input.TemplateID != nil && input.TemplateID.Valid && (len(input.Blocks) == 0 || string(input.Blocks) == "[]") {
		tpl, err := s.store.GetTemplate(ctx, *input.TemplateID, tenantID)
		if err == nil && len(tpl.Blocks) > 0 {
			input.Blocks = tpl.Blocks
		}
	}

	input.Blocks = sanitizeBlocksJSON(input.Blocks)

	return s.store.CreatePage(ctx, tenantID, createdBy, input)
}

// UpdatePage updates an existing page with input validation.
func (s *Service) UpdatePage(ctx context.Context, id, tenantID pgtype.UUID, input UpdatePageInput) (Page, error) {
	if input.Title == "" {
		return Page{}, validationError("title is required")
	}

	if input.Slug == "" {
		input.Slug = slugify(input.Title)
	}

	input.Blocks = sanitizeBlocksJSON(input.Blocks)

	return s.store.UpdatePage(ctx, id, tenantID, input)
}

// sanitizeBlocksJSON walks a JSON structure recursively and applies
// HTML sanitization to any string values. This prevents XSS via
// unsanitized user-supplied page builder content (AUDIT-BE-008).
func sanitizeBlocksJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}

	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}

	sanitized := sanitizeJSONValue(v)
	result, err := json.Marshal(sanitized)
	if err != nil {
		return raw
	}

	return result
}

func sanitizeJSONValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		for k, elem := range val {
			val[k] = sanitizeJSONValue(elem)
		}
		return val
	case []any:
		for i, elem := range val {
			val[i] = sanitizeJSONValue(elem)
		}
		return val
	case string:
		return content.SanitizeHTML(val)
	default:
		return v
	}
}

// DeletePage removes a page.
func (s *Service) DeletePage(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.DeletePage(ctx, id, tenantID)
}

// PublishPage sets the page status to 'published' and stamps published_at.
func (s *Service) PublishPage(ctx context.Context, id, tenantID pgtype.UUID) (Page, error) {
	return s.store.PublishPage(ctx, id, tenantID)
}

// ArchivePage sets the page status to 'archived'.
func (s *Service) ArchivePage(ctx context.Context, id, tenantID pgtype.UUID) (Page, error) {
	return s.store.ArchivePage(ctx, id, tenantID)
}

// ReorderBlocks updates the order of blocks for a page. It preserves the
// page's existing metadata (title/slug/status/theme/template/meta) and only
// replaces the blocks array. The prior implementation called store.UpdatePage
// with just Blocks set, which wiped every other column on each reorder
// (§1.3 data-loss bug).
func (s *Service) ReorderBlocks(ctx context.Context, id, tenantID pgtype.UUID, blocks []map[string]any) (Page, error) {
	blocksJSON, err := json.Marshal(blocks)
	if err != nil {
		return Page{}, fmt.Errorf("marshaling blocks: %w", err)
	}

	existing, err := s.store.GetPage(ctx, id, tenantID)
	if err != nil {
		return Page{}, err
	}

	return s.store.UpdatePage(ctx, id, tenantID, UpdatePageInput{
		Title:       existing.Title,
		Slug:        existing.Slug,
		Description: existing.Description,
		TemplateID:  existing.TemplateID,
		ThemeID:     existing.ThemeID,
		Blocks:      blocksJSON,
		Meta:        existing.Meta,
		Status:      existing.Status,
	})
}
