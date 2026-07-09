// Package tags — general cross-domain tags (Sections 08, 49).
//
// Distinct from forum_tags (forum-scoped, internal/forum): this package
// backs the tags table content is tagged against via content_tags, and
// followed via the follows package (entity_type="tag").
package tags

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Service provides tag read and admin-write access.
type Service struct {
	read  *gen.Queries
	write *gen.Queries
}

// NewService creates a Service backed by the given database.
func NewService(database *db.DB) *Service {
	return &Service{read: gen.New(database.Reader), write: gen.New(database.Writer)}
}

// ListTags returns every tag for the tenant, alphabetically.
func (s *Service) ListTags(ctx context.Context, tenantID pgtype.UUID) ([]gen.Tag, error) {
	tags, err := s.read.ListTagsByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}

	return tags, nil
}

// GetTagBySlug returns a single tag by its slug.
func (s *Service) GetTagBySlug(ctx context.Context, tenantID pgtype.UUID, slug string) (gen.Tag, error) {
	tag, err := s.read.GetTagBySlug(ctx, gen.GetTagBySlugParams{TenantID: tenantID, Slug: slug})
	if err != nil {
		return gen.Tag{}, fmt.Errorf("getting tag by slug: %w", err)
	}

	return tag, nil
}

// CreateTagInput holds parameters for creating a tag (catalog: POST /admin/tags).
type CreateTagInput struct {
	Name        string
	Slug        string
	Description *string
	Color       *string
}

// CreateTag creates a new tag.
func (s *Service) CreateTag(ctx context.Context, tenantID pgtype.UUID, input CreateTagInput) (gen.Tag, error) {
	tag, err := s.write.CreateTag(ctx, gen.CreateTagParams{
		TenantID:    tenantID,
		Name:        input.Name,
		Slug:        input.Slug,
		Description: input.Description,
		Color:       input.Color,
	})
	if err != nil {
		return gen.Tag{}, fmt.Errorf("creating tag: %w", err)
	}

	return tag, nil
}

// UpdateTagInput holds parameters for updating a tag (catalog: PATCH /admin/tags/:id).
type UpdateTagInput struct {
	Name        string
	Description *string
	Color       *string
}

// UpdateTag updates an existing tag's name/description/color. Slug is immutable.
func (s *Service) UpdateTag(ctx context.Context, id, tenantID pgtype.UUID, input UpdateTagInput) (gen.Tag, error) {
	tag, err := s.write.UpdateTag(ctx, gen.UpdateTagParams{
		ID:          id,
		TenantID:    tenantID,
		Name:        input.Name,
		Description: input.Description,
		Color:       input.Color,
	})
	if err != nil {
		return gen.Tag{}, fmt.Errorf("updating tag: %w", err)
	}

	return tag, nil
}

// DeleteTag deletes a tag (catalog: DELETE /admin/tags/:id).
func (s *Service) DeleteTag(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteTag(ctx, gen.DeleteTagParams{ID: id, TenantID: tenantID}); err != nil {
		return fmt.Errorf("deleting tag: %w", err)
	}

	return nil
}
