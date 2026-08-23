package docs

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for documentation pages.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
}

// NewStore creates a Store backed by the given gen.Queries instances.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// getBySlug returns a published doc page by audience and slug.
func (s *Store) getBySlug(ctx context.Context, tenantID pgtype.UUID, audience, slug string) (gen.DocPage, error) {
	row, err := s.read.GetDocPageBySlug(ctx, gen.GetDocPageBySlugParams{
		TenantID: tenantID,
		Audience: audience,
		Slug:     slug,
	})
	if err != nil {
		return gen.DocPage{}, fmt.Errorf("getting doc page by slug: %w", err)
	}

	return gen.DocPage{
		ID:             row.ID,
		TenantID:       row.TenantID,
		Title:          row.Title,
		Slug:           row.Slug,
		Audience:       row.Audience,
		Content:        row.Content,
		AiGenerated:    row.AiGenerated,
		ReviewStatus:   row.ReviewStatus,
		LastReviewedBy: row.LastReviewedBy,
		LastReviewedAt: row.LastReviewedAt,
		ParentID:       row.ParentID,
		Position:       row.Position,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}, nil
}

// getByID returns a doc page by ID.
func (s *Store) getByID(ctx context.Context, tenantID, id pgtype.UUID) (gen.DocPage, error) {
	row, err := s.read.GetDocPageByID(ctx, gen.GetDocPageByIDParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.DocPage{}, fmt.Errorf("getting doc page by ID: %w", err)
	}

	return gen.DocPage{
		ID:             row.ID,
		TenantID:       row.TenantID,
		Title:          row.Title,
		Slug:           row.Slug,
		Audience:       row.Audience,
		Content:        row.Content,
		AiGenerated:    row.AiGenerated,
		ReviewStatus:   row.ReviewStatus,
		LastReviewedBy: row.LastReviewedBy,
		LastReviewedAt: row.LastReviewedAt,
		ParentID:       row.ParentID,
		Position:       row.Position,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}, nil
}

// listByAudience returns published doc pages for an audience.
func (s *Store) listByAudience(ctx context.Context, tenantID pgtype.UUID, audience string) ([]gen.ListDocPagesByAudienceRow, error) {
	rows, err := s.read.ListDocPagesByAudience(ctx, gen.ListDocPagesByAudienceParams{
		TenantID: tenantID,
		Audience: audience,
	})
	if err != nil {
		return nil, fmt.Errorf("listing doc pages by audience: %w", err)
	}

	return rows, nil
}

// listAllByAudience returns all doc pages (any status) for an audience.
func (s *Store) listAllByAudience(ctx context.Context, tenantID pgtype.UUID, audience string) ([]gen.ListAllDocPagesByAudienceRow, error) {
	rows, err := s.read.ListAllDocPagesByAudience(ctx, gen.ListAllDocPagesByAudienceParams{
		TenantID: tenantID,
		Audience: audience,
	})
	if err != nil {
		return nil, fmt.Errorf("listing all doc pages by audience: %w", err)
	}

	return rows, nil
}

// listAll returns all doc pages (any status, any audience) for admin listing.
func (s *Store) listAll(ctx context.Context, tenantID pgtype.UUID) ([]gen.ListAllDocPagesRow, error) {
	rows, err := s.read.ListAllDocPages(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing all doc pages: %w", err)
	}

	return rows, nil
}

// listPublished returns all published doc pages across audiences.
func (s *Store) listPublished(ctx context.Context, tenantID pgtype.UUID) ([]gen.ListPublishedDocPagesRow, error) {
	rows, err := s.read.ListPublishedDocPages(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing published doc pages: %w", err)
	}

	return rows, nil
}

// search returns published doc pages matching the query by title or content.
// When audience is non-nil, results are filtered to that audience section.
func (s *Store) search(ctx context.Context, tenantID pgtype.UUID, query string, audience *string, limit int32) ([]gen.SearchDocPagesRow, error) {
	rows, err := s.read.SearchDocPages(ctx, gen.SearchDocPagesParams{
		TenantID:    tenantID,
		Audience:    audience,
		Query:       query,
		ResultLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("searching doc pages: %w", err)
	}

	return rows, nil
}

// create inserts a new doc page and returns its ID.
func (s *Store) create(ctx context.Context, params gen.CreateDocPageParams) (pgtype.UUID, error) {
	id, err := s.write.CreateDocPage(ctx, params)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("creating doc page: %w", err)
	}

	return id, nil
}

// update modifies an existing doc page.
func (s *Store) update(ctx context.Context, params gen.UpdateDocPageParams) error {
	if err := s.write.UpdateDocPage(ctx, params); err != nil {
		return fmt.Errorf("updating doc page: %w", err)
	}

	return nil
}

// updateStatus updates only the review status and reviewer of a doc page.
func (s *Store) updateStatus(ctx context.Context, params gen.UpdateDocPageStatusParams) error {
	if err := s.write.UpdateDocPageStatus(ctx, params); err != nil {
		return fmt.Errorf("updating doc page status: %w", err)
	}

	return nil
}

// getStatus returns the current review_status of a doc page.
func (s *Store) getStatus(ctx context.Context, params gen.GetDocPageStatusParams) (string, error) {
	status, err := s.read.GetDocPageStatus(ctx, params)
	if err != nil {
		return "", fmt.Errorf("getting doc page status: %w", err)
	}

	return status, nil
}

// delete removes a doc page.
func (s *Store) delete(ctx context.Context, tenantID, id pgtype.UUID) error {
	if err := s.write.DeleteDocPage(ctx, gen.DeleteDocPageParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("deleting doc page: %w", err)
	}

	return nil
}

// incrementHelpful increments the aggregate helpful_yes / helpful_no counters.
func (s *Store) incrementHelpful(ctx context.Context, tenantID pgtype.UUID, audience, slug, vote string) error {
	if err := s.write.IncrementDocPageHelpful(ctx, gen.IncrementDocPageHelpfulParams{
		TenantID: tenantID,
		Audience: audience,
		Slug:     slug,
		Vote:     vote,
	}); err != nil {
		return fmt.Errorf("incrementing helpful counters: %w", err)
	}
	return nil
}
