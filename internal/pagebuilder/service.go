package pagebuilder

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
)

// Service provides pagebuilder business logic with reader/writer pool separation.
type Service struct {
	store *Store
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{store: NewStore(database)}
}

// ---------------------------------------------------------------------------
// Block operations
// ---------------------------------------------------------------------------

// ListBlocks returns all blocks for a tenant.
func (s *Service) ListBlocks(ctx context.Context, tenantID pgtype.UUID) ([]Block, error) {
	return s.store.ListBlocks(ctx, tenantID)
}

// GetBlock returns a single block by ID, scoped to the tenant.
func (s *Service) GetBlock(ctx context.Context, id, tenantID pgtype.UUID) (Block, error) {
	return s.store.GetBlock(ctx, id, tenantID)
}

// CreateBlock creates a new block with input validation.
func (s *Service) CreateBlock(ctx context.Context, tenantID pgtype.UUID, input CreateBlockInput) (Block, error) {
	if input.Name == "" {
		return Block{}, errors.New("name is required")
	}

	if input.Slug == "" {
		return Block{}, errors.New("slug is required")
	}

	if input.Component == "" {
		return Block{}, errors.New("component is required")
	}

	if input.Category == "" {
		input.Category = "general"
	}

	return s.store.CreateBlock(ctx, tenantID, input)
}

// UpdateBlock updates an existing block with input validation.
func (s *Service) UpdateBlock(ctx context.Context, id, tenantID pgtype.UUID, input UpdateBlockInput) (Block, error) {
	if input.Name == "" {
		return Block{}, errors.New("name is required")
	}

	if input.Component == "" {
		return Block{}, errors.New("component is required")
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

// ListTemplates returns all templates for a tenant.
func (s *Service) ListTemplates(ctx context.Context, tenantID pgtype.UUID) ([]Template, error) {
	return s.store.ListTemplates(ctx, tenantID)
}

// GetTemplate returns a single template by ID, scoped to the tenant.
func (s *Service) GetTemplate(ctx context.Context, id, tenantID pgtype.UUID) (Template, error) {
	return s.store.GetTemplate(ctx, id, tenantID)
}

// CreateTemplate creates a new template with input validation.
func (s *Service) CreateTemplate(ctx context.Context, tenantID pgtype.UUID, input CreateTemplateInput) (Template, error) {
	if input.Name == "" {
		return Template{}, errors.New("name is required")
	}

	if input.Slug == "" {
		return Template{}, errors.New("slug is required")
	}

	if input.PageType == "" {
		input.PageType = "landing"
	}

	return s.store.CreateTemplate(ctx, tenantID, input)
}

// UpdateTemplate updates an existing template with input validation.
func (s *Service) UpdateTemplate(ctx context.Context, id, tenantID pgtype.UUID, input UpdateTemplateInput) (Template, error) {
	if input.Name == "" {
		return Template{}, errors.New("name is required")
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

// ListThemes returns all themes for a tenant.
func (s *Service) ListThemes(ctx context.Context, tenantID pgtype.UUID) ([]Theme, error) {
	return s.store.ListThemes(ctx, tenantID)
}

// GetTheme returns a single theme by ID, scoped to the tenant.
func (s *Service) GetTheme(ctx context.Context, id, tenantID pgtype.UUID) (Theme, error) {
	return s.store.GetTheme(ctx, id, tenantID)
}

// CreateTheme creates a new theme with input validation.
func (s *Service) CreateTheme(ctx context.Context, tenantID pgtype.UUID, input CreateThemeInput) (Theme, error) {
	if input.Name == "" {
		return Theme{}, errors.New("name is required")
	}

	if input.Slug == "" {
		return Theme{}, errors.New("slug is required")
	}

	return s.store.CreateTheme(ctx, tenantID, input)
}

// UpdateTheme updates an existing theme with input validation.
func (s *Service) UpdateTheme(ctx context.Context, id, tenantID pgtype.UUID, input UpdateThemeInput) (Theme, error) {
	if input.Name == "" {
		return Theme{}, errors.New("name is required")
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

// ListPages returns all pages for a tenant.
func (s *Service) ListPages(ctx context.Context, tenantID pgtype.UUID) ([]Page, error) {
	return s.store.ListPages(ctx, tenantID)
}

// GetPage returns a single page by ID, scoped to the tenant.
func (s *Service) GetPage(ctx context.Context, id, tenantID pgtype.UUID) (Page, error) {
	return s.store.GetPage(ctx, id, tenantID)
}

// GetPageBySlug returns a published page by slug for public consumption.
func (s *Service) GetPageBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (Page, error) {
	return s.store.GetPageBySlug(ctx, slug, tenantID)
}

// CreatePage creates a new page with input validation.
func (s *Service) CreatePage(ctx context.Context, tenantID, createdBy pgtype.UUID, input CreatePageInput) (Page, error) {
	if input.Title == "" {
		return Page{}, errors.New("title is required")
	}

	if input.Slug == "" {
		return Page{}, errors.New("slug is required")
	}

	if input.Status == "" {
		input.Status = PageStatusDraft
	}

	return s.store.CreatePage(ctx, tenantID, createdBy, input)
}

// UpdatePage updates an existing page with input validation.
func (s *Service) UpdatePage(ctx context.Context, id, tenantID pgtype.UUID, input UpdatePageInput) (Page, error) {
	if input.Title == "" {
		return Page{}, errors.New("title is required")
	}

	return s.store.UpdatePage(ctx, id, tenantID, input)
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
