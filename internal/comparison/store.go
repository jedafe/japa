package comparison

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for comparison operations.
// The Service layer wraps Store methods with business logic.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
}

// NewStore creates a Store backed by the given gen.Queries instances.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// ---------------------------------------------------------------------------
// Comparison Type Definitions
// ---------------------------------------------------------------------------

//nolint:unused // reserved for future features
func (s *Store) createComparisonType(ctx context.Context, p gen.CreateComparisonTypeParams) (gen.CreateComparisonTypeRow, error) {
	row, err := s.write.CreateComparisonType(ctx, p)
	if err != nil {
		return gen.CreateComparisonTypeRow{}, fmt.Errorf("creating comparison type: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) getComparisonTypeByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetComparisonTypeByIDRow, error) {
	row, err := s.read.GetComparisonTypeByID(ctx, gen.GetComparisonTypeByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.GetComparisonTypeByIDRow{}, fmt.Errorf("getting comparison type by ID: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) getComparisonTypeBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.GetComparisonTypeBySlugRow, error) {
	row, err := s.read.GetComparisonTypeBySlug(ctx, gen.GetComparisonTypeBySlugParams{Slug: slug, TenantID: tenantID})
	if err != nil {
		return gen.GetComparisonTypeBySlugRow{}, fmt.Errorf("getting comparison type by slug: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) listComparisonTypesByTenant(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.ListComparisonTypesByTenantRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListComparisonTypesByTenant(ctx, gen.ListComparisonTypesByTenantParams{
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing comparison types: %w", err)
	}

	return rows, nil
}

//nolint:unused // reserved for future features
func (s *Store) updateComparisonType(ctx context.Context, p gen.UpdateComparisonTypeParams) (gen.UpdateComparisonTypeRow, error) {
	row, err := s.write.UpdateComparisonType(ctx, p)
	if err != nil {
		return gen.UpdateComparisonTypeRow{}, fmt.Errorf("updating comparison type: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) deleteComparisonType(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteComparisonType(ctx, gen.DeleteComparisonTypeParams{ID: id, TenantID: tenantID}); err != nil {
		return fmt.Errorf("deleting comparison type: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Comparison Type Access
// ---------------------------------------------------------------------------

//nolint:unused // reserved for future features
func (s *Store) setComparisonTypeAccess(ctx context.Context, p gen.SetComparisonTypeAccessParams) error {
	if err := s.write.SetComparisonTypeAccess(ctx, p); err != nil {
		return fmt.Errorf("setting comparison type access: %w", err)
	}

	return nil
}

//nolint:unused // reserved for future features
func (s *Store) listComparisonTypeAccess(ctx context.Context, typeID pgtype.UUID) ([]gen.ListComparisonTypeAccessRow, error) {
	rows, err := s.read.ListComparisonTypeAccess(ctx, typeID)
	if err != nil {
		return nil, fmt.Errorf("listing comparison type access: %w", err)
	}

	return rows, nil
}

//nolint:unused // reserved for future features
func (s *Store) deleteComparisonTypeAccess(ctx context.Context, typeID, tenantID, userGroupID pgtype.UUID) error {
	if err := s.write.DeleteComparisonTypeAccess(ctx, gen.DeleteComparisonTypeAccessParams{
		TypeID:      typeID,
		TenantID:    tenantID,
		UserGroupID: userGroupID,
	}); err != nil {
		return fmt.Errorf("deleting comparison type access: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Comparison Items
// ---------------------------------------------------------------------------

//nolint:unused // reserved for future features
func (s *Store) createComparisonItem(ctx context.Context, p gen.CreateComparisonItemParams) (gen.CreateComparisonItemRow, error) {
	row, err := s.write.CreateComparisonItem(ctx, p)
	if err != nil {
		return gen.CreateComparisonItemRow{}, fmt.Errorf("creating comparison item: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) listComparisonItemsByContent(ctx context.Context, contentID, tenantID pgtype.UUID) ([]gen.ListComparisonItemsByContentRow, error) {
	rows, err := s.read.ListComparisonItemsByContent(ctx, gen.ListComparisonItemsByContentParams{
		ContentID: contentID,
		TenantID:  tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing comparison items: %w", err)
	}

	return rows, nil
}

//nolint:unused // reserved for future features
func (s *Store) getComparisonItemByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetComparisonItemByIDRow, error) {
	row, err := s.read.GetComparisonItemByID(ctx, gen.GetComparisonItemByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.GetComparisonItemByIDRow{}, fmt.Errorf("getting comparison item: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) updateComparisonItem(ctx context.Context, p gen.UpdateComparisonItemParams) (gen.UpdateComparisonItemRow, error) {
	row, err := s.write.UpdateComparisonItem(ctx, p)
	if err != nil {
		return gen.UpdateComparisonItemRow{}, fmt.Errorf("updating comparison item: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) deleteComparisonItem(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteComparisonItem(ctx, gen.DeleteComparisonItemParams{ID: id, TenantID: tenantID}); err != nil {
		return fmt.Errorf("deleting comparison item: %w", err)
	}

	return nil
}

//nolint:unused // reserved for future features
func (s *Store) reorderComparisonItems(ctx context.Context, id, contentID, tenantID pgtype.UUID, position int32) error {
	if err := s.write.ReorderComparisonItems(ctx, gen.ReorderComparisonItemsParams{
		ID:        id,
		ContentID: contentID,
		TenantID:  tenantID,
		Position:  position,
	}); err != nil {
		return fmt.Errorf("reordering comparison items: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Comparison Dimensions
// ---------------------------------------------------------------------------

//nolint:unused // reserved for future features
func (s *Store) createComparisonDimension(ctx context.Context, p gen.CreateComparisonDimensionParams) (gen.CreateComparisonDimensionRow, error) {
	row, err := s.write.CreateComparisonDimension(ctx, p)
	if err != nil {
		return gen.CreateComparisonDimensionRow{}, fmt.Errorf("creating comparison dimension: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) listComparisonDimensionsByContent(ctx context.Context, contentID, tenantID pgtype.UUID) ([]gen.ListComparisonDimensionsByContentRow, error) {
	rows, err := s.read.ListComparisonDimensionsByContent(ctx, gen.ListComparisonDimensionsByContentParams{
		ContentID: contentID,
		TenantID:  tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing comparison dimensions: %w", err)
	}

	return rows, nil
}

//nolint:unused // reserved for future features
func (s *Store) getComparisonDimensionByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetComparisonDimensionByIDRow, error) {
	row, err := s.read.GetComparisonDimensionByID(ctx, gen.GetComparisonDimensionByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.GetComparisonDimensionByIDRow{}, fmt.Errorf("getting comparison dimension: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) updateComparisonDimension(ctx context.Context, p gen.UpdateComparisonDimensionParams) (gen.UpdateComparisonDimensionRow, error) {
	row, err := s.write.UpdateComparisonDimension(ctx, p)
	if err != nil {
		return gen.UpdateComparisonDimensionRow{}, fmt.Errorf("updating comparison dimension: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) deleteComparisonDimension(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteComparisonDimension(ctx, gen.DeleteComparisonDimensionParams{ID: id, TenantID: tenantID}); err != nil {
		return fmt.Errorf("deleting comparison dimension: %w", err)
	}

	return nil
}

//nolint:unused // reserved for future features
func (s *Store) reorderComparisonDimensions(ctx context.Context, id, contentID, tenantID pgtype.UUID, position int32) error {
	if err := s.write.ReorderComparisonDimensions(ctx, gen.ReorderComparisonDimensionsParams{
		ID:        id,
		ContentID: contentID,
		TenantID:  tenantID,
		Position:  position,
	}); err != nil {
		return fmt.Errorf("reordering comparison dimensions: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Comparison Values
// ---------------------------------------------------------------------------

//nolint:unused // reserved for future features
func (s *Store) upsertComparisonValue(ctx context.Context, p gen.UpsertComparisonValueParams) (gen.ComparisonValue, error) {
	row, err := s.write.UpsertComparisonValue(ctx, p)
	if err != nil {
		return gen.ComparisonValue{}, fmt.Errorf("upserting comparison value: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) upsertComparisonValueWithMetadata(ctx context.Context, p gen.UpsertComparisonValueWithMetadataParams) (gen.ComparisonValue, error) {
	row, err := s.write.UpsertComparisonValueWithMetadata(ctx, p)
	if err != nil {
		return gen.ComparisonValue{}, fmt.Errorf("upserting comparison value with metadata: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) listComparisonValuesByDimension(ctx context.Context, dimensionID, tenantID pgtype.UUID) ([]gen.ListComparisonValuesByDimensionRow, error) {
	rows, err := s.read.ListComparisonValuesByDimension(ctx, gen.ListComparisonValuesByDimensionParams{
		DimensionID: dimensionID,
		TenantID:    tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing comparison values: %w", err)
	}

	return rows, nil
}

//nolint:unused // reserved for future features
func (s *Store) getComparisonValue(ctx context.Context, dimensionID, itemID, tenantID pgtype.UUID) (gen.ComparisonValue, error) {
	row, err := s.read.GetComparisonValue(ctx, gen.GetComparisonValueParams{
		DimensionID: dimensionID,
		ItemID:      itemID,
		TenantID:    tenantID,
	})
	if err != nil {
		return gen.ComparisonValue{}, fmt.Errorf("getting comparison value: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) deleteComparisonValuesByContent(ctx context.Context, contentID, tenantID pgtype.UUID) error {
	if err := s.write.DeleteComparisonValuesByContent(ctx, gen.DeleteComparisonValuesByContentParams{
		ContentID: contentID,
		TenantID:  tenantID,
	}); err != nil {
		return fmt.Errorf("deleting comparison values by content: %w", err)
	}

	return nil
}

//nolint:unused // reserved for future features
func (s *Store) deleteComparisonValuesByDimension(ctx context.Context, dimensionID, tenantID pgtype.UUID) error {
	if err := s.write.DeleteComparisonValuesByDimension(ctx, gen.DeleteComparisonValuesByDimensionParams{
		DimensionID: dimensionID,
		TenantID:    tenantID,
	}); err != nil {
		return fmt.Errorf("deleting comparison values by dimension: %w", err)
	}

	return nil
}

//nolint:unused // reserved for future features
func (s *Store) getComparisonValuesByContent(ctx context.Context, contentID, tenantID pgtype.UUID) ([]gen.ComparisonValue, error) {
	rows, err := s.read.GetComparisonValuesByContent(ctx, gen.GetComparisonValuesByContentParams{
		ContentID: contentID,
		TenantID:  tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("getting comparison values by content: %w", err)
	}

	return rows, nil
}

// ---------------------------------------------------------------------------
// Filtered list + in-use guard (Section 30, BUG-006/BUG-007)
// ---------------------------------------------------------------------------

// listComparisonsFiltered returns comparison content rows filtered by optional
// type_id and space_id, with cursor-based pagination.
func (s *Store) listComparisonsFiltered(ctx context.Context, p gen.ListComparisonsFilteredParams) ([]gen.ListComparisonsFilteredRow, error) {
	rows, err := s.read.ListComparisonsFiltered(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("listing comparisons filtered: %w", err)
	}
	return rows, nil
}

// countComparisonItemsByType returns the number of comparison_items linked to a
// given type_id — used by DeleteComparisonType to guard against in-use deletion.
func (s *Store) countComparisonItemsByType(ctx context.Context, typeID, tenantID pgtype.UUID) (int32, error) {
	count, err := s.read.CountComparisonItemsByType(ctx, gen.CountComparisonItemsByTypeParams{
		TypeID:   typeID,
		TenantID: tenantID,
	})
	if err != nil {
		return 0, fmt.Errorf("counting comparison items by type: %w", err)
	}
	return count, nil
}
