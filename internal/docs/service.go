package docs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Service provides business logic for documentation pages.
type Service struct {
	store *Store
}

// NewService creates a Service backed by the given database pools.
func NewService(database *db.DB) *Service {
	return &Service{
		store: &Store{
			read:  gen.New(database.Reader),
			write: gen.New(database.Writer),
		},
	}
}

// GetDocPageBySlug returns a published page by audience and slug.
func (s *Service) GetDocPageBySlug(ctx context.Context, tenantID pgtype.UUID, audience, slug string) (*DocPage, error) {
	page, err := s.store.getBySlug(ctx, tenantID, audience, slug)
	if err != nil {
		return nil, err
	}

	return toDocPage(page), nil
}

// GetDocPageByID returns a page by ID.
func (s *Service) GetDocPageByID(ctx context.Context, tenantID, id pgtype.UUID) (*DocPage, error) {
	page, err := s.store.getByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	return toDocPage(page), nil
}

// ListDocPagesByAudience returns published pages for an audience.
func (s *Service) ListDocPagesByAudience(ctx context.Context, tenantID pgtype.UUID, audience string) ([]DocPageSummary, error) {
	rows, err := s.store.listByAudience(ctx, tenantID, audience)
	if err != nil {
		return nil, err
	}

	summaries := make([]DocPageSummary, len(rows))
	for i, r := range rows {
		summaries[i] = DocPageSummary{
			ID:           r.ID.String(),
			Title:        r.Title,
			Slug:         r.Slug,
			Audience:     r.Audience,
			ReviewStatus: r.ReviewStatus,
			ParentID:     uuidPtr(r.ParentID),
			Position:     r.Position,
			CreatedAt:    r.CreatedAt.Time.Format(time.RFC3339),
		}
	}

	return summaries, nil
}

// ListAllDocPagesByAudience returns all pages (any status) for admin listing.
func (s *Service) ListAllDocPagesByAudience(ctx context.Context, tenantID pgtype.UUID, audience string) ([]DocPage, error) {
	rows, err := s.store.listAllByAudience(ctx, tenantID, audience)
	if err != nil {
		return nil, err
	}

	pages := make([]DocPage, len(rows))
	for i, r := range rows {
		pages[i] = DocPage{
			ID:             r.ID.String(),
			Title:          r.Title,
			Slug:           r.Slug,
			Audience:       r.Audience,
			AIGenerated:    r.AiGenerated,
			ReviewStatus:   r.ReviewStatus,
			LastReviewedBy: uuidPtr(r.LastReviewedBy),
			LastReviewedAt: timestamptzPtr(r.LastReviewedAt),
			ParentID:       uuidPtr(r.ParentID),
			Position:       r.Position,
			CreatedAt:      r.CreatedAt.Time.Format(time.RFC3339),
			UpdatedAt:      r.UpdatedAt.Time.Format(time.RFC3339),
		}
	}

	return pages, nil
}

// ListAllDocPages returns all pages (any status, any audience) for admin listing.
func (s *Service) ListAllDocPages(ctx context.Context, tenantID pgtype.UUID) ([]DocPage, error) {
	rows, err := s.store.listAll(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	pages := make([]DocPage, len(rows))
	for i, r := range rows {
		pages[i] = DocPage{
			ID:             r.ID.String(),
			Title:          r.Title,
			Slug:           r.Slug,
			Audience:       r.Audience,
			AIGenerated:    r.AiGenerated,
			ReviewStatus:   r.ReviewStatus,
			LastReviewedBy: uuidPtr(r.LastReviewedBy),
			LastReviewedAt: timestamptzPtr(r.LastReviewedAt),
			ParentID:       uuidPtr(r.ParentID),
			Position:       r.Position,
			CreatedAt:      r.CreatedAt.Time.Format(time.RFC3339),
			UpdatedAt:      r.UpdatedAt.Time.Format(time.RFC3339),
		}
	}

	return pages, nil
}

// GetDocNav returns the published-pages navigation tree grouped by audience.
func (s *Service) GetDocNav(ctx context.Context, tenantID pgtype.UUID) ([]DocNavTree, error) {
	rows, err := s.store.listPublished(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	return buildDocNavTree(rows), nil
}

// SearchDocPages returns published pages matching the query, with a plain-text excerpt.
func (s *Service) SearchDocPages(ctx context.Context, tenantID pgtype.UUID, query string, limit int32) ([]DocSearchResult, error) {
	rows, err := s.store.search(ctx, tenantID, query, limit)
	if err != nil {
		return nil, err
	}

	results := make([]DocSearchResult, len(rows))
	for i, r := range rows {
		results[i] = DocSearchResult{
			ID:       r.ID.String(),
			Title:    r.Title,
			Slug:     r.Slug,
			Audience: r.Audience,
			Excerpt:  extractExcerpt(r.Content, excerptMaxLen),
		}
	}

	return results, nil
}

// excerptMaxLen caps search excerpts at a sidebar-friendly length.
const excerptMaxLen = 140

// buildDocNavTree groups published pages by audience (in canonical order) and
// nests child pages one level deep under their parents. Children whose parent
// is not published are promoted to roots so they stay reachable.
func buildDocNavTree(rows []gen.ListPublishedDocPagesRow) []DocNavTree {
	byAudience := make(map[string][]gen.ListPublishedDocPagesRow)
	for _, r := range rows {
		byAudience[r.Audience] = append(byAudience[r.Audience], r)
	}

	trees := make([]DocNavTree, 0, len(byAudience))
	for _, audience := range AudienceOrder {
		group, ok := byAudience[audience]
		if !ok {
			continue
		}

		trees = append(trees, DocNavTree{Audience: audience, Items: buildAudienceNavItems(group)})
	}

	return trees
}

// buildAudienceNavItems nests one audience's pages by parent_id, preserving
// the position/title ordering of the input rows.
func buildAudienceNavItems(group []gen.ListPublishedDocPagesRow) []DocNavItem {
	roots := make([]DocNavItem, 0, len(group))
	rootIdx := make(map[string]int, len(group))

	for _, r := range group {
		if r.ParentID.Valid {
			continue
		}

		item := navItemFromRow(r)
		rootIdx[item.ID] = len(roots)
		roots = append(roots, item)
	}

	for _, r := range group {
		if !r.ParentID.Valid {
			continue
		}

		if idx, ok := rootIdx[r.ParentID.String()]; ok {
			roots[idx].Children = append(roots[idx].Children, navItemFromRow(r))
		} else {
			roots = append(roots, navItemFromRow(r))
		}
	}

	return roots
}

// navItemFromRow converts a published-page row to a nav item.
func navItemFromRow(r gen.ListPublishedDocPagesRow) DocNavItem {
	return DocNavItem{
		ID:       r.ID.String(),
		Title:    r.Title,
		Slug:     r.Slug,
		Audience: r.Audience,
		ParentID: uuidPtr(r.ParentID),
		Position: r.Position,
	}
}

// extractExcerpt walks TipTap-style JSON content and joins its text nodes
// into a single plain-text excerpt, truncated to maxLen runes.
func extractExcerpt(content []byte, maxLen int) string {
	var doc any
	if err := json.Unmarshal(content, &doc); err != nil {
		return ""
	}

	var b strings.Builder
	collectText(doc, &b)

	text := strings.Join(strings.Fields(b.String()), " ")

	runes := []rune(text)
	if len(runes) > maxLen {
		text = string(runes[:maxLen]) + "…"
	}

	return text
}

// collectText recursively gathers "text" values from TipTap JSON nodes.
func collectText(node any, b *strings.Builder) {
	switch v := node.(type) {
	case map[string]any:
		if s, ok := v["text"].(string); ok {
			b.WriteString(s)
			b.WriteString(" ")
		}

		if c, ok := v["content"]; ok {
			collectText(c, b)
		}
	case []any:
		for _, item := range v {
			collectText(item, b)
		}
	}
}

// CreateDocPage inserts a new documentation page.
func (s *Service) CreateDocPage(ctx context.Context, tenantID pgtype.UUID, req CreateDocPageRequest) error {
	if !ValidAudiences[req.Audience] {
		return fmt.Errorf("invalid audience: %s", req.Audience)
	}

	status := req.ReviewStatus
	if status == "" {
		status = "draft"
	}

	if !ValidReviewStatuses[status] {
		return fmt.Errorf("invalid review_status: %s", status)
	}

	contentBytes := []byte(req.Content)
	if len(contentBytes) == 0 {
		contentBytes = []byte(`""`)
	}

	var parentID pgtype.UUID
	if req.ParentID != nil {
		if err := parentID.Scan(*req.ParentID); err != nil {
			return fmt.Errorf("invalid parent_id %q: %w", *req.ParentID, err)
		}
	}

	return s.store.create(ctx, gen.CreateDocPageParams{
		TenantID:     tenantID,
		Title:        req.Title,
		Slug:         req.Slug,
		Audience:     req.Audience,
		Content:      contentBytes,
		AiGenerated:  req.AIGenerated,
		ReviewStatus: status,
		ParentID:     parentID,
		Position:     req.Position,
	})
}

// UpdateDocPage modifies an existing documentation page.
func (s *Service) UpdateDocPage(ctx context.Context, tenantID, id pgtype.UUID, req UpdateDocPageRequest) error {
	status := req.ReviewStatus
	if status == "" {
		status = "draft"
	}

	if !ValidReviewStatuses[status] {
		return fmt.Errorf("invalid review_status: %s", status)
	}

	// Preserve existing content when the request doesn't include it (e.g. status-only toggle).
	contentBytes := req.Content
	if contentBytes == nil {
		existing, err := s.store.getByID(ctx, tenantID, id)
		if err != nil {
			return fmt.Errorf("fetching existing page for content merge: %w", err)
		}
		contentBytes = existing.Content
	} else {
		var err error
		contentBytes, err = json.Marshal(string(req.Content))
		if err != nil {
			return fmt.Errorf("marshaling content: %w", err)
		}
	}

	return s.store.update(ctx, gen.UpdateDocPageParams{
		ID:             id,
		TenantID:       tenantID,
		Title:          req.Title,
		Content:        contentBytes,
		ReviewStatus:   status,
		LastReviewedBy: pgtype.UUID{},
	})
}

// DeleteDocPage removes a documentation page.
func (s *Service) DeleteDocPage(ctx context.Context, tenantID, id pgtype.UUID) error {
	return s.store.delete(ctx, tenantID, id)
}

// toDocPage converts a gen.DocPage to our API type.
func toDocPage(p gen.DocPage) *DocPage {
	var content json.RawMessage
	if len(p.Content) > 0 {
		content = p.Content
	}

	return &DocPage{
		ID:             p.ID.String(),
		Title:          p.Title,
		Slug:           p.Slug,
		Audience:       p.Audience,
		Content:        content,
		AIGenerated:    p.AiGenerated,
		ReviewStatus:   p.ReviewStatus,
		LastReviewedBy: uuidPtr(p.LastReviewedBy),
		LastReviewedAt: timestamptzPtr(p.LastReviewedAt),
		ParentID:       uuidPtr(p.ParentID),
		Position:       p.Position,
		CreatedAt:      p.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:      p.UpdatedAt.Time.Format(time.RFC3339),
	}
}

// uuidPtr returns a string pointer from a pgtype.UUID, or nil if the UUID is zero.
func uuidPtr(u pgtype.UUID) *string {
	if !u.Valid {
		return nil
	}

	s := u.String()

	return &s
}

// timestamptzPtr returns a formatted time string pointer, or nil if not valid.
func timestamptzPtr(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}

	s := t.Time.Format(time.RFC3339)

	return &s
}
