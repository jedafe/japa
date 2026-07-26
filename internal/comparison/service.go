package comparison

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Comparison Type Definitions
// ---------------------------------------------------------------------------

// CreateComparisonTypeInput holds parameters for creating a comparison type.
type CreateComparisonTypeInput struct {
	Name         string
	Slug         string
	Description  string
	Icon         string
	FieldGroupID pgtype.UUID
	ItemLabel    string
	IsSystem     bool
}

// CreateComparisonType creates a new admin-defined comparison type.
func (s *Service) CreateComparisonType(ctx context.Context, tenantID pgtype.UUID, input CreateComparisonTypeInput) (gen.ComparisonTypeDefinition, error) {
	if input.Name == "" {
		return gen.ComparisonTypeDefinition{}, errors.New("name is required")
	}

	if input.Slug == "" {
		return gen.ComparisonTypeDefinition{}, errors.New("slug is required")
	}

	if input.ItemLabel == "" {
		input.ItemLabel = "Item"
	}

	var description *string
	if input.Description != "" {
		description = &input.Description
	}

	var icon *string
	if input.Icon != "" {
		icon = &input.Icon
	}

	return s.store.createComparisonType(ctx, gen.CreateComparisonTypeParams{
		TenantID:     tenantID,
		Name:         input.Name,
		Slug:         input.Slug,
		Description:  description,
		Icon:         icon,
		FieldGroupID: input.FieldGroupID,
		ItemLabel:    input.ItemLabel,
		IsSystem:     input.IsSystem,
	})
}

// GetComparisonType retrieves a comparison type by ID.
func (s *Service) GetComparisonType(ctx context.Context, id, tenantID pgtype.UUID) (gen.ComparisonTypeDefinition, error) {
	return s.store.getComparisonTypeByID(ctx, id, tenantID)
}

// GetComparisonTypeBySlug retrieves a comparison type by slug.
func (s *Service) GetComparisonTypeBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.ComparisonTypeDefinition, error) {
	return s.store.getComparisonTypeBySlug(ctx, slug, tenantID)
}

// ListComparisonTypes returns all comparison types for a tenant.
func (s *Service) ListComparisonTypes(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.ComparisonTypeDefinition, error) {
	return s.store.listComparisonTypesByTenant(ctx, tenantID, limit, offset)
}

// CreateComparisonInput holds parameters for creating a comparison.
type CreateComparisonInput struct {
	Title      string
	Body       string
	Visibility string
	SpaceID    pgtype.UUID
}

// CreateComparisonResult holds the result of creating a comparison.
type CreateComparisonResult struct {
	ID         pgtype.UUID `json:"id"`
	TenantID   pgtype.UUID `json:"tenant_id"`
	AuthorID   pgtype.UUID `json:"author_id"`
	Title      string      `json:"title"`
	Slug       string      `json:"slug"`
	Visibility string      `json:"visibility"`
	SpaceID    pgtype.UUID `json:"space_id"`
}

// CreateComparison creates a comparison by inserting a content row with
// content_type='comparison'. The comparison items, dimensions, and values
// are then linked to this content row via its ID.
func (s *Service) CreateComparison(ctx context.Context, tenantID, userID pgtype.UUID, input CreateComparisonInput) (CreateComparisonResult, error) {
	if input.Title == "" {
		return CreateComparisonResult{}, errors.New("title is required")
	}

	visibility := input.Visibility
	if visibility == "" {
		visibility = content.VisibilityMembers
	}

	contentRow, err := s.contentSvc.CreateContent(ctx, tenantID, content.CreateContentInput{
		AuthorID:     userID,
		SpaceID:      input.SpaceID,
		ContentType:  content.TypeComparison,
		Title:        input.Title,
		Body:         input.Body,
		SeoIndexable: true, // default: indexable — matches the column DEFAULT
		Status:       content.StatusPublished,
		Visibility:   visibility,
	})
	if err != nil {
		return CreateComparisonResult{}, fmt.Errorf("creating comparison content: %w", err)
	}

	return CreateComparisonResult{
		ID:         contentRow.ID,
		TenantID:   contentRow.TenantID,
		AuthorID:   contentRow.AuthorID,
		Title:      contentRow.Title,
		Slug:       contentRow.Slug,
		Visibility: contentRow.Visibility,
		SpaceID:    contentRow.SpaceID,
	}, nil
}

// UpdateComparisonTypeInput holds parameters for updating a comparison type.
type UpdateComparisonTypeInput struct {
	Name        string
	Description string
	Icon        string
	ItemLabel   string
}

// UpdateComparisonType updates an existing comparison type.
func (s *Service) UpdateComparisonType(ctx context.Context, id, tenantID pgtype.UUID, input UpdateComparisonTypeInput) (gen.ComparisonTypeDefinition, error) {
	if input.Name == "" {
		return gen.ComparisonTypeDefinition{}, errors.New("name is required")
	}

	var description *string
	if input.Description != "" {
		description = &input.Description
	}

	var icon *string
	if input.Icon != "" {
		icon = &input.Icon
	}

	return s.store.updateComparisonType(ctx, gen.UpdateComparisonTypeParams{
		ID:          id,
		TenantID:    tenantID,
		Name:        input.Name,
		Description: description,
		Icon:        icon,
		ItemLabel:   input.ItemLabel,
	})
}

// DeleteComparisonType removes a comparison type and its cascaded children.
func (s *Service) DeleteComparisonType(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.deleteComparisonType(ctx, id, tenantID)
}

// ---------------------------------------------------------------------------
// Comparison Type Access
// ---------------------------------------------------------------------------

// SetComparisonTypeAccessInput holds parameters for setting type access.
type SetComparisonTypeAccessInput struct {
	UserGroupID pgtype.UUID
	CanCreate   bool
	CanFill     bool
	AICanFill   bool
}

// SetComparisonTypeAccess upserts access permissions for a user group on a type.
func (s *Service) SetComparisonTypeAccess(ctx context.Context, typeID pgtype.UUID, input SetComparisonTypeAccessInput) error {
	return s.store.setComparisonTypeAccess(ctx, gen.SetComparisonTypeAccessParams{
		TypeID:      typeID,
		UserGroupID: input.UserGroupID,
		CanCreate:   input.CanCreate,
		CanFill:     input.CanFill,
		AiCanFill:   input.AICanFill,
	})
}

// ListComparisonTypeAccess returns all access entries for a type.
func (s *Service) ListComparisonTypeAccess(ctx context.Context, typeID pgtype.UUID) ([]gen.ListComparisonTypeAccessRow, error) {
	return s.store.listComparisonTypeAccess(ctx, typeID)
}

// DeleteComparisonTypeAccess removes an access entry for a user group.
func (s *Service) DeleteComparisonTypeAccess(ctx context.Context, typeID, userGroupID pgtype.UUID) error {
	return s.store.deleteComparisonTypeAccess(ctx, typeID, userGroupID)
}

// ---------------------------------------------------------------------------
// Comparison Items
// ---------------------------------------------------------------------------

// CreateComparisonItemInput holds parameters for creating a comparison item.
type CreateComparisonItemInput struct {
	ContentID   pgtype.UUID
	Title       string
	Description string
	ImageID     pgtype.UUID
	GeoSpaceID  pgtype.UUID
	Position    int32
}

// CreateComparisonItem adds a new item to a comparison.
func (s *Service) CreateComparisonItem(ctx context.Context, tenantID pgtype.UUID, input CreateComparisonItemInput) (gen.ComparisonItem, error) {
	if input.Title == "" {
		return gen.ComparisonItem{}, errors.New("title is required")
	}

	var description *string
	if input.Description != "" {
		description = &input.Description
	}

	return s.store.createComparisonItem(ctx, gen.CreateComparisonItemParams{
		ContentID:   input.ContentID,
		TenantID:    tenantID,
		Title:       input.Title,
		Description: description,
		ImageID:     input.ImageID,
		GeoSpaceID:  input.GeoSpaceID,
		Position:    input.Position,
	})
}

// ListComparisonItems returns all items for a comparison.
func (s *Service) ListComparisonItems(ctx context.Context, contentID, tenantID pgtype.UUID) ([]gen.ComparisonItem, error) {
	return s.store.listComparisonItemsByContent(ctx, contentID, tenantID)
}

// UpdateComparisonItemInput holds parameters for updating a comparison item.
type UpdateComparisonItemInput struct {
	Title       string
	Description string
	ImageID     pgtype.UUID
	GeoSpaceID  pgtype.UUID
	Position    int32
}

// UpdateComparisonItem updates an existing comparison item.
func (s *Service) UpdateComparisonItem(ctx context.Context, id, tenantID pgtype.UUID, input UpdateComparisonItemInput) (gen.ComparisonItem, error) {
	if input.Title == "" {
		return gen.ComparisonItem{}, errors.New("title is required")
	}

	var description *string
	if input.Description != "" {
		description = &input.Description
	}

	return s.store.updateComparisonItem(ctx, gen.UpdateComparisonItemParams{
		ID:          id,
		TenantID:    tenantID,
		Title:       input.Title,
		Description: description,
		ImageID:     input.ImageID,
		GeoSpaceID:  input.GeoSpaceID,
		Position:    input.Position,
	})
}

// DeleteComparisonItem removes a comparison item.
func (s *Service) DeleteComparisonItem(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.deleteComparisonItem(ctx, id, tenantID)
}

// ReorderComparisonItem changes the position of a comparison item.
func (s *Service) ReorderComparisonItem(ctx context.Context, id, contentID, tenantID pgtype.UUID, position int32) error {
	return s.store.reorderComparisonItems(ctx, id, contentID, tenantID, position)
}

// ---------------------------------------------------------------------------
// Comparison Dimensions
// ---------------------------------------------------------------------------

// CreateComparisonDimensionInput holds parameters for creating a dimension.
type CreateComparisonDimensionInput struct {
	ContentID     pgtype.UUID
	Label         string
	DimensionType string
	Position      int32
}

// CreateComparisonDimension adds a new dimension to a comparison.
func (s *Service) CreateComparisonDimension(ctx context.Context, tenantID pgtype.UUID, input CreateComparisonDimensionInput) (gen.ComparisonDimension, error) {
	if input.Label == "" {
		return gen.ComparisonDimension{}, errors.New("label is required")
	}

	if !IsValidDimensionType(input.DimensionType) {
		return gen.ComparisonDimension{}, fmt.Errorf("invalid dimension type: %s", input.DimensionType)
	}

	return s.store.createComparisonDimension(ctx, gen.CreateComparisonDimensionParams{
		ContentID:     input.ContentID,
		TenantID:      tenantID,
		Label:         input.Label,
		DimensionType: input.DimensionType,
		Position:      input.Position,
	})
}

// ListComparisonDimensions returns all dimensions for a comparison.
func (s *Service) ListComparisonDimensions(ctx context.Context, contentID, tenantID pgtype.UUID) ([]gen.ComparisonDimension, error) {
	return s.store.listComparisonDimensionsByContent(ctx, contentID, tenantID)
}

// UpdateComparisonDimensionInput holds parameters for updating a dimension.
type UpdateComparisonDimensionInput struct {
	Label         string
	DimensionType string
	Position      int32
}

// UpdateComparisonDimension updates an existing dimension.
func (s *Service) UpdateComparisonDimension(ctx context.Context, id, tenantID pgtype.UUID, input UpdateComparisonDimensionInput) (gen.ComparisonDimension, error) {
	if input.Label == "" {
		return gen.ComparisonDimension{}, errors.New("label is required")
	}

	if !IsValidDimensionType(input.DimensionType) {
		return gen.ComparisonDimension{}, fmt.Errorf("invalid dimension type: %s", input.DimensionType)
	}

	return s.store.updateComparisonDimension(ctx, gen.UpdateComparisonDimensionParams{
		ID:            id,
		TenantID:      tenantID,
		Label:         input.Label,
		DimensionType: input.DimensionType,
		Position:      input.Position,
	})
}

// DeleteComparisonDimension removes a comparison dimension and its values.
func (s *Service) DeleteComparisonDimension(ctx context.Context, id, tenantID pgtype.UUID) error {
	// Delete values first (FK cascade handles this, but explicit for clarity).
	if err := s.store.deleteComparisonValuesByDimension(ctx, id, tenantID); err != nil {
		return fmt.Errorf("deleting dimension values: %w", err)
	}

	return s.store.deleteComparisonDimension(ctx, id, tenantID)
}

// ReorderComparisonDimension changes the position of a dimension.
func (s *Service) ReorderComparisonDimension(ctx context.Context, id, contentID, tenantID pgtype.UUID, position int32) error {
	return s.store.reorderComparisonDimensions(ctx, id, contentID, tenantID, position)
}

// ---------------------------------------------------------------------------
// Comparison Values
// ---------------------------------------------------------------------------

// UpsertComparisonValueInput holds parameters for upserting a value.
type UpsertComparisonValueInput struct {
	DimensionID  pgtype.UUID
	ItemID       pgtype.UUID
	Value        string
	ValueRating  *float64
	ValueBoolean *bool
}

// UpsertComparisonValue creates or updates a value at a dimension×item intersection.
func (s *Service) UpsertComparisonValue(ctx context.Context, tenantID pgtype.UUID, input UpsertComparisonValueInput) (gen.ComparisonValue, error) {
	var value *string
	if input.Value != "" {
		value = &input.Value
	}

	var valueRating pgtype.Numeric
	if input.ValueRating != nil {
		if err := valueRating.Scan(fmt.Sprintf("%v", *input.ValueRating)); err != nil {
			return gen.ComparisonValue{}, fmt.Errorf("invalid value_rating: %w", err)
		}
	}

	return s.store.upsertComparisonValue(ctx, gen.UpsertComparisonValueParams{
		DimensionID:  input.DimensionID,
		ItemID:       input.ItemID,
		TenantID:     tenantID,
		Value:        value,
		ValueRating:  valueRating,
		ValueBoolean: input.ValueBoolean,
	})
}

// ListComparisonValuesByDimension returns all values for a dimension with item titles.
func (s *Service) ListComparisonValuesByDimension(ctx context.Context, dimensionID, tenantID pgtype.UUID) ([]gen.ListComparisonValuesByDimensionRow, error) {
	return s.store.listComparisonValuesByDimension(ctx, dimensionID, tenantID)
}

// GetComparisonValue returns a single value at a dimension×item intersection.
func (s *Service) GetComparisonValue(ctx context.Context, dimensionID, itemID, tenantID pgtype.UUID) (gen.ComparisonValue, error) {
	return s.store.getComparisonValue(ctx, dimensionID, itemID, tenantID)
}

// ---------------------------------------------------------------------------
// Comparison Matrix (assembled view)
// ---------------------------------------------------------------------------

// ComparisonMatrixItem represents a single item in the comparison matrix.
type ComparisonMatrixItem struct { //nolint:revive // stuttering type name preserved for API compatibility
	Item       gen.ComparisonItem          `json:"item"`
	Dimensions []ComparisonMatrixDimension `json:"dimensions"`
}

// ComparisonMatrixDimension represents a dimension value for an item in the matrix.
type ComparisonMatrixDimension struct { //nolint:revive // stuttering type name preserved for API compatibility
	DimensionID  pgtype.UUID `json:"dimension_id"`
	Label        string      `json:"label"`
	Type         string      `json:"type"`
	Value        *string     `json:"value"`
	ValueRating  *float64    `json:"value_rating"`
	ValueBoolean *bool       `json:"value_boolean"`
}

// GetComparisonMatrix assembles the full items×dimensions matrix for a comparison.
func (s *Service) GetComparisonMatrix(ctx context.Context, contentID, tenantID pgtype.UUID) ([]ComparisonMatrixItem, error) {
	items, err := s.store.listComparisonItemsByContent(ctx, contentID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing items for matrix: %w", err)
	}

	dimensions, err := s.store.listComparisonDimensionsByContent(ctx, contentID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing dimensions for matrix: %w", err)
	}

	// Fetch all values in a single query instead of M×N individual lookups.
	values, err := s.store.getComparisonValuesByContent(ctx, contentID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing values for matrix: %w", err)
	}

	// Index values by (itemID, dimensionID) for O(1) lookup.
	type valueKey struct {
		itemID      [16]byte
		dimensionID [16]byte
	}

	valueIndex := make(map[valueKey]gen.ComparisonValue, len(values))
	for _, v := range values {
		valueIndex[valueKey{v.ItemID.Bytes, v.DimensionID.Bytes}] = v
	}

	matrix := make([]ComparisonMatrixItem, 0, len(items))

	for _, item := range items {
		mi := ComparisonMatrixItem{Item: item, Dimensions: make([]ComparisonMatrixDimension, 0, len(dimensions))}

		for _, dim := range dimensions {
			md := ComparisonMatrixDimension{
				DimensionID: dim.ID,
				Label:       dim.Label,
				Type:        dim.DimensionType,
			}
			if val, ok := valueIndex[valueKey{item.ID.Bytes, dim.ID.Bytes}]; ok {
				md.Value = val.Value

				if val.ValueRating.Valid {
					var r float64
					if err := val.ValueRating.Scan(&r); err == nil {
						md.ValueRating = &r
					}
				}

				md.ValueBoolean = val.ValueBoolean
			}

			mi.Dimensions = append(mi.Dimensions, md)
		}

		matrix = append(matrix, mi)
	}

	return matrix, nil
}

// ListComparisons returns a page of comparison content rows for the tenant.
// Comparisons are content rows with content_type='comparison'; they carry no
// comparison-type linkage in the schema, so no type filter is available here.
func (s *Service) ListComparisons(ctx context.Context, tenantID pgtype.UUID, limit int32) ([]gen.ListContentsByTypeRow, error) {
	return s.contentSvc.ListContentsByType(ctx, tenantID, content.TypeComparison, pgtype.Timestamptz{}, limit)
}
