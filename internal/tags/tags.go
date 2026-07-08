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

// Service provides tag read access.
type Service struct {
	read *gen.Queries
}

// NewService creates a Service backed by the given database.
func NewService(database *db.DB) *Service {
	return &Service{read: gen.New(database.Reader)}
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
