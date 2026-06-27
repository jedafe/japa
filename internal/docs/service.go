package docs

import (
	"context"
	"encoding/json"
	"fmt"

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
			CreatedAt:    r.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
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
			CreatedAt:      r.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:      r.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
		}
	}
	return pages, nil
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
	contentBytes, err := json.Marshal(req.Content)
	if err != nil {
		return fmt.Errorf("marshaling content: %w", err)
	}
	var parentID pgtype.UUID
	if req.ParentID != nil {
		_ = parentID.Scan(*req.ParentID)
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
	contentBytes, err := json.Marshal(req.Content)
	if err != nil {
		return fmt.Errorf("marshaling content: %w", err)
	}
	return s.store.update(ctx, gen.UpdateDocPageParams{
		ID:            id,
		TenantID:      tenantID,
		Title:         req.Title,
		Content:       contentBytes,
		ReviewStatus:  status,
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
		CreatedAt:      p.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:      p.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
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
	s := t.Time.Format("2006-01-02T15:04:05Z")
	return &s
}
