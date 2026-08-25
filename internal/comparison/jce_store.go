package comparison

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// JCE metadata store helpers
// ---------------------------------------------------------------------------

func (s *Store) insertComparisonContentMetadata(ctx context.Context, p gen.InsertComparisonContentMetadataParams) error {
	if err := s.write.InsertComparisonContentMetadata(ctx, p); err != nil {
		return fmt.Errorf("inserting comparison content metadata: %w", err)
	}
	return nil
}

func (s *Store) getComparisonContentMetadata(ctx context.Context, contentID, tenantID pgtype.UUID) (gen.ComparisonContentMetadatum, error) {
	row, err := s.read.GetComparisonContentMetadata(ctx, gen.GetComparisonContentMetadataParams{
		ContentID: contentID,
		TenantID:  tenantID,
	})
	if err != nil {
		return gen.ComparisonContentMetadatum{}, fmt.Errorf("getting comparison content metadata: %w", err)
	}
	return row, nil
}

func (s *Store) updateComparisonContentMetadata(ctx context.Context, p gen.UpdateComparisonContentMetadataParams) error {
	if err := s.write.UpdateComparisonContentMetadata(ctx, p); err != nil {
		return fmt.Errorf("updating comparison content metadata: %w", err)
	}
	return nil
}

func (s *Store) deleteComparisonContentMetadata(ctx context.Context, contentID, tenantID pgtype.UUID) error {
	if err := s.write.DeleteComparisonContentMetadata(ctx, gen.DeleteComparisonContentMetadataParams{
		ContentID: contentID,
		TenantID:  tenantID,
	}); err != nil {
		return fmt.Errorf("deleting comparison content metadata: %w", err)
	}
	return nil
}

func (s *Store) listComparisonsWithFilters(ctx context.Context, p gen.ListComparisonsWithFiltersParams) ([]gen.ListComparisonsWithFiltersRow, error) {
	rows, err := s.read.ListComparisonsWithFilters(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("listing comparisons with filters: %w", err)
	}
	return rows, nil
}

func (s *Store) countComparisonsWithFilters(ctx context.Context, p gen.CountComparisonsWithFiltersParams) (int32, error) {
	count, err := s.read.CountComparisonsWithFilters(ctx, p)
	if err != nil {
		return 0, fmt.Errorf("counting comparisons with filters: %w", err)
	}
	return count, nil
}

func (s *Store) getComparisonBySlugOrID(ctx context.Context, id pgtype.UUID, slug *string, tenantID pgtype.UUID) (gen.GetComparisonBySlugOrIDRow, error) {
	row, err := s.read.GetComparisonBySlugOrID(ctx, gen.GetComparisonBySlugOrIDParams{
		ID:       id,
		Slug:     slug,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.GetComparisonBySlugOrIDRow{}, fmt.Errorf("getting comparison by slug or ID: %w", err)
	}
	return row, nil
}
