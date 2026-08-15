package pagebuilder

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// Service provides pagebuilder business logic with reader/writer pool separation.
type Service struct {
	store *Store
	redis valkeycompat.Cmdable
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{store: NewStore(database)}
}

// SetRedis allows late binding of the Valkey client (matching the pattern used
// by other services constructed before the Valkey client is ready). Used for
// the published-page cache (jc:page:{tenant_id}:{slug}).
func (s *Service) SetRedis(r valkeycompat.Cmdable) {
	s.redis = r
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
func (s *Service) ListBlocks(ctx context.Context, tenantID pgtype.UUID, q string, page, limit int32) ([]Block, int64, error) {
	return s.store.ListBlocks(ctx, tenantID, q, page, limit)
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
func (s *Service) ListTemplates(ctx context.Context, tenantID pgtype.UUID, q, category string, page, limit int32) ([]Template, int64, error) {
	return s.store.ListTemplates(ctx, tenantID, q, category, page, limit)
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
func (s *Service) ListThemes(ctx context.Context, tenantID pgtype.UUID, q string, page, limit int32) ([]Theme, int64, error) {
	return s.store.ListThemes(ctx, tenantID, q, page, limit)
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

// ListPages returns a paginated list of pages for a tenant, optionally
// narrowed by f. page is 1-based; limit is the page size.
func (s *Service) ListPages(ctx context.Context, tenantID pgtype.UUID, f PageListFilter, page, limit int32) ([]Page, int64, error) {
	return s.store.ListPages(ctx, tenantID, f, page, limit)
}

// ListPublishedPages returns only published pages for public consumption.
func (s *Service) ListPublishedPages(ctx context.Context, tenantID pgtype.UUID) ([]Page, error) {
	return s.store.ListPublishedPages(ctx, tenantID)
}

// GetPage returns a single page by ID, scoped to the tenant.
func (s *Service) GetPage(ctx context.Context, id, tenantID pgtype.UUID) (Page, error) {
	return s.store.GetPage(ctx, id, tenantID)
}

// GetPageBySlug returns a published page by slug for public consumption.
// Results are Valkey-cached under jc:page:{tenant_id}:{slug} with a TTL from
// the page.cache_ttl_secs system setting (default 300s). The cache is
// invalidated on any page write (update/delete/publish/archive).
// Ref: conventions/valkey-keys-catalog.md — jc:page:{tenant_id}:{slug}
func (s *Service) GetPageBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (Page, error) {
	cacheKey := pageCacheKey(tenantID, slug)

	// Try Valkey cache first.
	if s.redis != nil {
		if cached, err := s.redis.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var p Page
			if err := json.Unmarshal([]byte(cached), &p); err == nil {
				return p, nil
			}
		}
	}

	p, err := s.store.GetPageBySlug(ctx, slug, tenantID)
	if err != nil {
		return Page{}, err
	}

	// Cache the published page.
	if s.redis != nil {
		if serialized, err := json.Marshal(p); err == nil {
			_ = s.redis.Set(ctx, cacheKey, string(serialized), 300*time.Second)
		}
	}

	return p, nil
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

// DuplicatePage clones an existing page into a new draft. The copy inherits
// the source's blocks, meta, theme, and page_type so an admin can redesign it
// for another app's homepage (e.g. duplicate the default guest landing page
// and reshape it for Learn/Docs). The new slug is derived from the source
// slug with a "-copy" suffix (incremented until unique) and the title gets a
// " (Copy)" suffix. The copy always starts as a draft so it is never
// accidentally published over the source.
func (s *Service) DuplicatePage(ctx context.Context, id, tenantID, createdBy pgtype.UUID) (Page, error) {
	src, err := s.store.GetPage(ctx, id, tenantID)
	if err != nil {
		return Page{}, err
	}

	// Derive a tenant-unique slug: {src.Slug}-copy, -copy-2, -copy-3, ...
	// Bounded to 1000 attempts to guarantee termination even if a tenant has
	// somehow filled the -copy-N namespace.
	base := src.Slug
	if base == "" {
		base = "page"
	}
	slug := base + "-copy"
	for n := 2; n <= 1000; n++ {
		exists, err := s.store.SlugExists(ctx, slug, tenantID)
		if err != nil {
			return Page{}, err
		}
		if !exists {
			break
		}
		slug = fmt.Sprintf("%s-copy-%d", base, n)
		if n == 1000 {
			return Page{}, validationError("could not find a unique slug for the duplicate after 1000 attempts")
		}
	}

	input := CreatePageInput{
		Title:    src.Title + " (Copy)",
		Slug:     slug,
		Blocks:   src.Blocks,
		Meta:     src.Meta,
		ThemeID:  src.ThemeID,
		PageType: src.PageType,
		Status:   PageStatusDraft,
	}
	// template_id is intentionally not copied: the duplicate is an
	// independent page instance, not a template derivative.

	return s.CreatePage(ctx, tenantID, createdBy, input)
}

// UpdatePage updates an existing page with input validation.
func (s *Service) UpdatePage(ctx context.Context, id, tenantID pgtype.UUID, input UpdatePageInput) (Page, error) {
	if input.Title == "" {
		return Page{}, validationError("title is required")
	}

	// NOTE: unlike CreatePage, we do NOT auto-derive the slug from the title
	// here. Auto-deriving on every save would overwrite a user's custom slug
	// whenever the client omits the slug field (the editor only sends title +
	// blocks). The store preserves the existing slug when input.Slug is empty.

	input.Blocks = sanitizeBlocksJSON(input.Blocks)

	// Fetch the existing slug for cache invalidation before the update.
	existing, _ := s.store.GetPage(ctx, id, tenantID)

	p, err := s.store.UpdatePage(ctx, id, tenantID, input)
	if err != nil {
		return Page{}, err
	}

	// Invalidate cache for both the old and new slug (slug may have changed).
	s.invalidatePageCache(ctx, tenantID, existing.Slug)
	s.invalidatePageCache(ctx, tenantID, p.Slug)

	return p, nil
}

// sanitizeBlocksJSON scopes HTML sanitization to the fields that actually
// carry HTML: the `content` block's `props.html` string (the TipTap-backed
// rich-text block per ADR-011). Blanket-sanitizing every string in the block
// array corrupts non-HTML props such as URLs (href, background_image),
// plain-text labels (eyebrow, subhead, button labels), and alt text. Ref:
// §14 Tiptap sanitize; ADR-011; AUDIT-BE-008.
func sanitizeBlocksJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}

	var blocks []map[string]any
	if err := json.Unmarshal(raw, &blocks); err != nil {
		// Not a block array (e.g. a single object) — leave untouched.
		return raw
	}

	for _, block := range blocks {
		blockType, _ := block["type"].(string)
		if blockType != "content" {
			continue
		}
		props, ok := block["props"].(map[string]any)
		if !ok {
			continue
		}
		if html, ok := props["html"].(string); ok {
			props["html"] = content.SanitizeHTML(html)
		}
	}

	result, err := json.Marshal(blocks)
	if err != nil {
		return raw
	}

	return result
}

// DeletePage removes a page and invalidates its cache.
func (s *Service) DeletePage(ctx context.Context, id, tenantID pgtype.UUID) error {
	// Fetch the slug for cache invalidation before deletion.
	existing, _ := s.store.GetPage(ctx, id, tenantID)

	if err := s.store.DeletePage(ctx, id, tenantID); err != nil {
		return err
	}

	s.invalidatePageCache(ctx, tenantID, existing.Slug)
	return nil
}

// PublishPage sets the page status to 'published' and stamps published_at.
func (s *Service) PublishPage(ctx context.Context, id, tenantID pgtype.UUID) (Page, error) {
	p, err := s.store.PublishPage(ctx, id, tenantID)
	if err != nil {
		return Page{}, err
	}
	s.invalidatePageCache(ctx, tenantID, p.Slug)
	return p, nil
}

// ArchivePage sets the page status to 'archived'.
func (s *Service) ArchivePage(ctx context.Context, id, tenantID pgtype.UUID) (Page, error) {
	p, err := s.store.ArchivePage(ctx, id, tenantID)
	if err != nil {
		return Page{}, err
	}
	s.invalidatePageCache(ctx, tenantID, p.Slug)
	return p, nil
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
		PageType:    existing.PageType,
	})
}

// invalidatePageCache deletes the Valkey cache entry for a published page.
// Ref: conventions/valkey-keys-catalog.md — jc:page:{tenant_id}:{slug}
func (s *Service) invalidatePageCache(ctx context.Context, tenantID pgtype.UUID, slug string) {
	if s.redis == nil || slug == "" {
		return
	}
	_ = s.redis.Del(ctx, pageCacheKey(tenantID, slug)).Err()
}

// pageCacheKey returns the Valkey key for a published page's cached data.
// Ref: conventions/valkey-keys-catalog.md — jc:page:{tenant_id}:{slug}
func pageCacheKey(tenantID pgtype.UUID, slug string) string {
	return "jc:page:" + tenantID.String() + ":" + slug
}
