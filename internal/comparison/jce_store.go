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

func (s *Store) getComparisonContentMetadata(ctx context.Context, contentID pgtype.UUID) (gen.ComparisonContentMetadatum, error) {
	row, err := s.read.GetComparisonContentMetadata(ctx,
		contentID,
	)
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

func (s *Store) deleteComparisonContentMetadata(ctx context.Context, contentID pgtype.UUID) error {
	if err := s.write.DeleteComparisonContentMetadata(ctx,
		contentID,
	); err != nil {
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

func (s *Store) countComparisonsByType(ctx context.Context, p gen.CountComparisonsByTypeParams) ([]gen.CountComparisonsByTypeRow, error) {
	rows, err := s.read.CountComparisonsByType(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("counting comparisons by type: %w", err)
	}

	return rows, nil
}

func (s *Store) listComparisonItemPreviews(ctx context.Context, contentIDs []pgtype.UUID) ([]gen.ListComparisonItemPreviewsRow, error) {
	rows, err := s.read.ListComparisonItemPreviews(ctx, contentIDs)
	if err != nil {
		return nil, fmt.Errorf("listing comparison item previews: %w", err)
	}

	return rows, nil
}

func (s *Store) listSpaceGeoByIDs(ctx context.Context, ids []pgtype.UUID) ([]gen.ListSpaceGeoByIDsRow, error) {
	rows, err := s.read.ListSpaceGeoByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("listing space geo codes: %w", err)
	}

	return rows, nil
}

func (s *Store) getMediaByIDs(ctx context.Context, ids []pgtype.UUID) ([]gen.Medium, error) {
	rows, err := s.read.GetMediaByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("getting media by ids: %w", err)
	}

	return rows, nil
}

func (s *Store) getComparisonBySlugOrID(ctx context.Context, id pgtype.UUID, slug *string) (gen.GetComparisonBySlugOrIDRow, error) {
	row, err := s.read.GetComparisonBySlugOrID(ctx, gen.GetComparisonBySlugOrIDParams{
		ID:   id,
		Slug: slug,
	})
	if err != nil {
		return gen.GetComparisonBySlugOrIDRow{}, fmt.Errorf("getting comparison by slug or ID: %w", err)
	}

	return row, nil
}

// ---------------------------------------------------------------------------
// Dimension pool helpers (§30 Sub-spec 6)
// ---------------------------------------------------------------------------

func (s *Store) listComparisonTypeFieldGroups(ctx context.Context, typeID pgtype.UUID) ([]gen.ListComparisonTypeFieldGroupsRow, error) {
	rows, err := s.read.ListComparisonTypeFieldGroups(ctx, typeID)
	if err != nil {
		return nil, fmt.Errorf("listing comparison type field groups: %w", err)
	}

	return rows, nil
}

func (s *Store) listComparisonTypePoolFields(ctx context.Context, typeID pgtype.UUID) ([]gen.ListComparisonTypePoolFieldsRow, error) {
	rows, err := s.read.ListComparisonTypePoolFields(ctx, typeID)
	if err != nil {
		return nil, fmt.Errorf("listing comparison type pool fields: %w", err)
	}

	return rows, nil
}

func (s *Store) listPoolFieldsByGroupIDs(ctx context.Context, typeID pgtype.UUID, ids []pgtype.UUID) ([]gen.ListPoolFieldsByGroupIDsRow, error) {
	rows, err := s.read.ListPoolFieldsByGroupIDs(ctx, gen.ListPoolFieldsByGroupIDsParams{
		TypeID:   typeID,
		GroupIds: ids,
	})
	if err != nil {
		return nil, fmt.Errorf("listing pool fields by groups: %w", err)
	}

	return rows, nil
}

func (s *Store) countPoolFieldsByIDs(ctx context.Context, typeID pgtype.UUID, ids []pgtype.UUID) (int64, error) {
	n, err := s.read.CountPoolFieldsByIDs(ctx, gen.CountPoolFieldsByIDsParams{
		TypeID:   typeID,
		FieldIds: ids,
	})
	if err != nil {
		return 0, fmt.Errorf("counting pool fields: %w", err)
	}

	return n, nil
}

func (s *Store) listCustomFieldsByIDs(ctx context.Context, ids []pgtype.UUID) ([]gen.ListCustomFieldsByIDsRow, error) {
	rows, err := s.read.ListCustomFieldsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("listing custom fields: %w", err)
	}

	return rows, nil
}
