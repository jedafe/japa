package comparison

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
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
func (s *Service) CreateComparisonType(ctx context.Context, tenantID pgtype.UUID, input CreateComparisonTypeInput) (gen.CreateComparisonTypeRow, error) {
	if input.Name == "" {
		return gen.CreateComparisonTypeRow{}, errors.New("name is required")
	}

	if input.Slug == "" {
		return gen.CreateComparisonTypeRow{}, errors.New("slug is required")
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
func (s *Service) GetComparisonType(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetComparisonTypeByIDRow, error) {
	return s.store.getComparisonTypeByID(ctx, id, tenantID)
}

// GetComparisonTypeBySlug retrieves a comparison type by slug.
func (s *Service) GetComparisonTypeBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.GetComparisonTypeBySlugRow, error) {
	return s.store.getComparisonTypeBySlug(ctx, slug, tenantID)
}

// ComparisonTypeDimension is the dimension (custom field) projection shown in
// the admin Comparison Type detail view. Each field in the type's field group
// becomes one comparison dimension (Section 30).
type ComparisonTypeDimension struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	FieldType   string `json:"field_type"`
}

// ComparisonTypeDetail is the enriched type response returned by
// GET /comparisons/types/{slug}. It augments the raw type definition with the
// field-group dimensions and the item count so the admin detail panel can
// render real data instead of placeholders (Section 30 — "field count,
// comparison count").
type ComparisonTypeDetail struct {
	gen.GetComparisonTypeBySlugRow
	Dimensions []ComparisonTypeDimension `json:"dimensions"`
	ItemCount  int32                     `json:"item_count"`
}

// GetComparisonTypeDetail returns a comparison type by slug together with its
// dimensions (projected from the linked custom field group's fields) and the
// count of comparison items linked to the type.
//
// Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
func (s *Service) GetComparisonTypeDetail(ctx context.Context, slug string, tenantID pgtype.UUID) (ComparisonTypeDetail, error) {
	typeDef, err := s.store.getComparisonTypeBySlug(ctx, slug, tenantID)
	if err != nil {
		return ComparisonTypeDetail{}, err
	}

	dimensions := make([]ComparisonTypeDimension, 0)
	if typeDef.FieldGroupID.Valid {
		fields, err := s.cfSvc.ListFieldsByGroup(ctx, typeDef.FieldGroupID, tenantID)
		if err != nil {
			return ComparisonTypeDetail{}, fmt.Errorf("listing type dimensions: %w", err)
		}
		for _, f := range fields {
			dimensions = append(dimensions, ComparisonTypeDimension{
				ID:          f.ID.String(),
				Name:        f.Label,
				Description: f.Key,
				FieldType:   f.FieldType,
			})
		}
	}

	itemCount, err := s.store.countComparisonItemsByType(ctx, typeDef.ID, tenantID)
	if err != nil {
		return ComparisonTypeDetail{}, fmt.Errorf("counting comparison items: %w", err)
	}

	return ComparisonTypeDetail{
		GetComparisonTypeBySlugRow: typeDef,
		Dimensions:                 dimensions,
		ItemCount:                  itemCount,
	}, nil
}

// ComparisonTypeListItem augments a type definition with aggregate counts for
// the admin list view (Section 30 — "name, slug, field count, comparison
// count, enabled/disabled").
type ComparisonTypeListItem struct {
	gen.ListComparisonTypesByTenantRow
	FieldCount int32 `json:"field_count"`
	ItemCount  int32 `json:"item_count"`
}

// ListComparisonTypesWithCounts returns all comparison types for a tenant,
// each enriched with its field count (dimensions) and item count.
//
// Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
func (s *Service) ListComparisonTypesWithCounts(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]ComparisonTypeListItem, error) {
	types, err := s.store.listComparisonTypesByTenant(ctx, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}

	items := make([]ComparisonTypeListItem, 0, len(types))
	for _, td := range types {
		var fieldCount int32
		if td.FieldGroupID.Valid {
			fields, err := s.cfSvc.ListFieldsByGroup(ctx, td.FieldGroupID, tenantID)
			if err != nil {
				return nil, fmt.Errorf("listing fields for type %s: %w", td.Slug, err)
			}
			fieldCount = int32(len(fields))
		}
		itemCount, err := s.store.countComparisonItemsByType(ctx, td.ID, tenantID)
		if err != nil {
			return nil, fmt.Errorf("counting items for type %s: %w", td.Slug, err)
		}
		items = append(items, ComparisonTypeListItem{
			ListComparisonTypesByTenantRow: td,
			FieldCount:                     fieldCount,
			ItemCount:                      itemCount,
		})
	}
	return items, nil
}

// ListComparisonTypes returns all comparison types for a tenant.
func (s *Service) ListComparisonTypes(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.ListComparisonTypesByTenantRow, error) {
	return s.store.listComparisonTypesByTenant(ctx, tenantID, limit, offset)
}

// ComparisonItemInput is a single item passed by the create wizard.
type ComparisonItemInput struct {
	Name string
	Slug string
}

// CreateComparisonInput holds parameters for creating a comparison.
type CreateComparisonInput struct {
	Title      string
	Body       string
	Visibility string
	Status     string // 'draft' or 'published' (default 'published')
	SpaceID    pgtype.UUID
	TypeID     pgtype.UUID // optional: links the comparison to a comparison_type_definition
	Items      []ComparisonItemInput
	Dimensions []string // dimension labels (from the wizard's step 3)
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

// fieldTypeToDimensionType maps a custom_field field_type to a comparison
// dimension_type. Unknown field types default to text.
func fieldTypeToDimensionType(fieldType string) string {
	switch fieldType {
	case "number", "integer":
		return DimensionTypeNumber
	case "currency":
		return DimensionTypeCurrency
	case "boolean", "checkbox":
		return DimensionTypeBoolean
	case "rating", "select":
		return DimensionTypeRating
	default:
		return DimensionTypeText
	}
}

// CreateComparison creates a comparison by inserting a content row with
// content_type='comparison', then creates the items and dimensions supplied by
// the wizard. When a type_id is supplied, the type's field-group fields are
// auto-converted into dimensions (Section 30: "each field = one
// comparison_dimension row") unless the caller already supplied dimensions.
//
// Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
func (s *Service) CreateComparison(ctx context.Context, tenantID, userID pgtype.UUID, input CreateComparisonInput) (CreateComparisonResult, error) {
	if input.Title == "" {
		return CreateComparisonResult{}, errors.New("title is required")
	}

	visibility := input.Visibility
	if visibility == "" {
		visibility = content.VisibilityMembers
	}

	// Default to published for backward compat; the /compare/new wizard
	// passes 'draft' so the user can fill cell values in the /create editor
	// before publishing (Section 30 — "Publish (standard content publish flow)").
	status := input.Status
	if status == "" {
		status = content.StatusPublished
	}

	// Resolve the comparison type (if supplied) BEFORE the transaction so the
	// read-only lookup doesn't hold a transaction lock (BUG-003/BUG-004).
	var typeDef *gen.GetComparisonTypeByIDRow
	if input.TypeID.Valid {
		td, err := s.store.getComparisonTypeByID(ctx, input.TypeID, tenantID)
		if err != nil {
			return CreateComparisonResult{}, fmt.Errorf("resolving comparison type: %w", err)
		}
		typeDef = &td
	}

	// Pre-fetch field-group fields (if auto-generating dimensions) outside the
	// transaction for the same reason — avoids holding a txn during cf reads.
	var autoFields []gen.ListCustomFieldsByGroupRow
	if len(input.Dimensions) == 0 && typeDef != nil && typeDef.FieldGroupID.Valid {
		fields, ferr := s.cfSvc.ListFieldsByGroup(ctx, typeDef.FieldGroupID, tenantID)
		if ferr != nil {
			return CreateComparisonResult{}, fmt.Errorf("listing field-group fields: %w", ferr)
		}
		autoFields = fields
	}

	// All writes (content row + items + dimensions) run in a single
	// transaction so a failure partway through rolls back the entire
	// comparison — no orphaned content rows without items/dimensions.
	// Ref: §6.2 transaction boundary, BUG-001.
	return db.WithTxResult(ctx, s.db.Writer, func(q *gen.Queries) (CreateComparisonResult, error) {
		contentRow, err := s.contentSvc.CreateContentWithQueries(ctx, q, tenantID, content.CreateContentInput{
			AuthorID:     userID,
			SpaceID:      input.SpaceID,
			ContentType:  content.TypeComparison,
			Title:        input.Title,
			Body:         input.Body,
			SeoIndexable: true, // default: indexable — matches the column DEFAULT
			Status:       status,
			Visibility:   visibility,
		})
		if err != nil {
			return CreateComparisonResult{}, fmt.Errorf("creating comparison content: %w", err)
		}

		// Create items supplied by the wizard (BUG-002).
		for i, item := range input.Items {
			if item.Name == "" {
				continue
			}
			_, err := q.CreateComparisonItem(ctx, gen.CreateComparisonItemParams{
				ContentID: contentRow.ID,
				TenantID:  tenantID,
				TypeID:    input.TypeID,
				Title:     item.Name,
				Position:  int32(i),
			})
			if err != nil {
				return CreateComparisonResult{}, fmt.Errorf("creating comparison item %d: %w", i, err)
			}
		}

		// Auto-generate dimensions from the type's field group (Section 30).
		if autoFields != nil {
			for i, f := range autoFields {
				_, derr := q.CreateComparisonDimension(ctx, gen.CreateComparisonDimensionParams{
					ContentID:     contentRow.ID,
					TenantID:      tenantID,
					TypeID:        input.TypeID,
					Label:         f.Label,
					DimensionType: fieldTypeToDimensionType(f.FieldType),
					Position:      int32(i),
				})
				if derr != nil {
					return CreateComparisonResult{}, fmt.Errorf("auto-creating dimension %d: %w", i, derr)
				}
			}
		} else {
			// Use caller-supplied dimension labels.
			for i, label := range input.Dimensions {
				if label == "" {
					continue
				}
				_, err := q.CreateComparisonDimension(ctx, gen.CreateComparisonDimensionParams{
					ContentID:     contentRow.ID,
					TenantID:      tenantID,
					TypeID:        input.TypeID,
					Label:         label,
					DimensionType: DimensionTypeText,
					Position:      int32(i),
				})
				if err != nil {
					return CreateComparisonResult{}, fmt.Errorf("creating comparison dimension %d: %w", i, err)
				}
			}
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
	})
}

// UpdateComparisonInput holds parameters for updating a comparison's metadata.
// Only the parent content row's metadata is editable here; items, dimensions,
// and values have their own endpoints.
type UpdateComparisonInput struct {
	Title      string
	Slug       string
	Body       string
	Visibility string
	EditedBy   pgtype.UUID
}

// UpdateComparison updates the metadata of the content row backing a comparison
// (PATCH /api/v1/comparisons/{id}). It delegates to the content service so the
// standard edit-window, slug-uniqueness, and revision-creation rules apply.
// Only the author may update their comparison unless bypassOwnership is true
// (set when the caller has comparison.edit_any, BUG-002/BUG-011).
//
// Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
func (s *Service) UpdateComparison(ctx context.Context, id, tenantID pgtype.UUID, input UpdateComparisonInput, bypassOwnership bool) (gen.UpdateContentRow, error) {
	if !bypassOwnership {
		existing, err := s.contentSvc.GetContent(ctx, id, tenantID)
		if err != nil {
			return gen.UpdateContentRow{}, fmt.Errorf("getting comparison for ownership check: %w", err)
		}
		if existing.AuthorID.Valid && existing.AuthorID.Bytes != input.EditedBy.Bytes {
			return gen.UpdateContentRow{}, ErrNotOwner
		}
	}

	visibility := input.Visibility
	body := input.Body

	return s.contentSvc.UpdateContent(ctx, id, tenantID, input.EditedBy, content.UpdateContentInput{
		Title:      input.Title,
		Slug:       input.Slug,
		Body:       body,
		Visibility: visibility,
	}, bypassOwnership)
}

// DeleteComparison soft-deletes the content row backing a comparison
// (DELETE /api/v1/comparisons/{id}). Per Rule 11, only the content row is
// soft-deleted in V1; the linked comparison_items/dimensions/values remain
// (hidden because list queries filter on contents.deleted_at IS NULL) and are
// hard-removed only by the account:deletion_cleanup Asynq task.
// Only the author may delete their comparison unless bypassOwnership is true
// (set when the caller has comparison.delete_any, BUG-002/BUG-011).
//
// Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
func (s *Service) DeleteComparison(ctx context.Context, id, tenantID, deletedBy pgtype.UUID, bypassOwnership bool) error {
	if !bypassOwnership {
		existing, err := s.contentSvc.GetContent(ctx, id, tenantID)
		if err != nil {
			return fmt.Errorf("getting comparison for ownership check: %w", err)
		}
		if existing.AuthorID.Valid && existing.AuthorID.Bytes != deletedBy.Bytes {
			return ErrNotOwner
		}
	}

	return s.contentSvc.DeleteContent(ctx, id, tenantID)
}

// ComparisonDetail is the full detail view for a single comparison: the parent
// content metadata plus its items and dimensions.
type ComparisonDetail struct {
	ID         pgtype.UUID                                `json:"id"`
	TenantID   pgtype.UUID                                `json:"tenant_id"`
	AuthorID   pgtype.UUID                                `json:"author_id"`
	Title      string                                     `json:"title"`
	Slug       string                                     `json:"slug"`
	Body       string                                     `json:"body"`
	Visibility string                                     `json:"visibility"`
	Status     string                                     `json:"status"`
	SpaceID    pgtype.UUID                                `json:"space_id"`
	Items      []gen.ListComparisonItemsByContentRow      `json:"items"`
	Dimensions []gen.ListComparisonDimensionsByContentRow `json:"dimensions"`
}

// GetComparisonDetail returns the parent content metadata together with the
// comparison's items and dimensions (GET /api/v1/comparisons/{id}). The earlier
// handler returned only items+dimensions and dropped the title/slug/author the
// detail page needs.
//
// Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
func (s *Service) GetComparisonDetail(ctx context.Context, id, tenantID pgtype.UUID) (ComparisonDetail, error) {
	contentRow, err := s.contentSvc.GetContent(ctx, id, tenantID)
	if err != nil {
		return ComparisonDetail{}, fmt.Errorf("getting comparison content: %w", err)
	}

	items, err := s.store.listComparisonItemsByContent(ctx, id, tenantID)
	if err != nil {
		return ComparisonDetail{}, fmt.Errorf("listing comparison items: %w", err)
	}

	dimensions, err := s.store.listComparisonDimensionsByContent(ctx, id, tenantID)
	if err != nil {
		return ComparisonDetail{}, fmt.Errorf("listing comparison dimensions: %w", err)
	}

	return ComparisonDetail{
		ID:         contentRow.ID,
		TenantID:   contentRow.TenantID,
		AuthorID:   contentRow.AuthorID,
		Title:      contentRow.Title,
		Slug:       contentRow.Slug,
		Body:       contentRow.Body,
		Visibility: contentRow.Visibility,
		Status:     contentRow.Status,
		SpaceID:    contentRow.SpaceID,
		Items:      items,
		Dimensions: dimensions,
	}, nil
}

// ResolveComparisonContentID resolves a comparison identifier that may be
// either a content UUID or a slug into the backing content UUID. UUIDs are
// tried first (fast path); if the input is not a valid UUID it is treated as
// a slug and looked up via the content service (Section 30 —
// GET /api/v1/comparisons/:slug).
//
// The slug path verifies that the resolved content row has
// content_type='comparison' so that a blog post or forum thread slug cannot
// leak through the comparison API (BUG-001).
//
// Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
func (s *Service) ResolveComparisonContentID(ctx context.Context, identifier string, tenantID pgtype.UUID) (pgtype.UUID, error) {
	// Fast path: identifier is a UUID.
	var id pgtype.UUID
	if err := id.Scan(identifier); err == nil {
		return id, nil
	}

	// Slug path: look up the content row by slug, scoped to the tenant.
	contentRow, err := s.contentSvc.GetContentBySlug(ctx, identifier, tenantID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("resolving comparison identifier %q: %w", identifier, err)
	}

	// Verify the resolved content is actually a comparison — without this
	// check, any content slug (blog, forum, etc.) would resolve as a
	// comparison and leak through this API (BUG-001).
	if contentRow.ContentType != content.TypeComparison {
		return pgtype.UUID{}, fmt.Errorf("resolving comparison identifier %q: %w", identifier, pgx.ErrNoRows)
	}

	return contentRow.ID, nil
}

// UpdateComparisonTypeInput holds parameters for updating a comparison type.
type UpdateComparisonTypeInput struct {
	Name         string
	Slug         string
	Description  string
	Icon         string
	FieldGroupID pgtype.UUID
	ItemLabel    string
}

// UpdateComparisonType updates an existing comparison type.
func (s *Service) UpdateComparisonType(ctx context.Context, id, tenantID pgtype.UUID, input UpdateComparisonTypeInput) (gen.UpdateComparisonTypeRow, error) {
	if input.Name == "" {
		return gen.UpdateComparisonTypeRow{}, errors.New("name is required")
	}

	var description *string
	if input.Description != "" {
		description = &input.Description
	}

	var icon *string
	if input.Icon != "" {
		icon = &input.Icon
	}

	var slug *string
	if input.Slug != "" {
		slug = &input.Slug
	}

	return s.store.updateComparisonType(ctx, gen.UpdateComparisonTypeParams{
		ID:           id,
		TenantID:     tenantID,
		Name:         input.Name,
		Slug:         slug,
		Description:  description,
		Icon:         icon,
		FieldGroupID: input.FieldGroupID,
		ItemLabel:    input.ItemLabel,
	})
}

// ErrComparisonTypeInUse is returned when a DeleteComparisonType is attempted
// on a type that still has comparison_items linked to it.
var ErrComparisonTypeInUse = errors.New("comparison type is in use by existing comparisons")

// ErrNotOwner is returned when a user attempts to update or delete a
// comparison they did not author. The route-level RBAC check gates on
// comparison.edit_own / comparison.delete_own, but the service must also
// verify ownership so a user with _own permission cannot modify another
// user's comparison. Ref: §5.3 RBAC escape, BUG-002.
var ErrNotOwner = errors.New("only the author can modify this comparison")

// VerifyComparisonOwnership checks that the given user is the author of the
// comparison identified by contentID, or that bypassOwnership is true (set
// when the caller has comparison.edit_any). Returns ErrNotOwner if the user
// is not the author and bypass is false. Ref: §5.3 RBAC escape, BUG-003.
func (s *Service) VerifyComparisonOwnership(ctx context.Context, contentID, userID, tenantID pgtype.UUID, bypassOwnership bool) error {
	if bypassOwnership {
		return nil
	}
	existing, err := s.contentSvc.GetContent(ctx, contentID, tenantID)
	if err != nil {
		return fmt.Errorf("getting comparison for ownership check: %w", err)
	}
	if existing.AuthorID.Valid && existing.AuthorID.Bytes != userID.Bytes {
		return ErrNotOwner
	}
	return nil
}

// VerifyComparisonItemOwnership resolves the parent comparison from an item
// ID and then verifies ownership. Ref: §5.3 RBAC escape, BUG-003.
func (s *Service) VerifyComparisonItemOwnership(ctx context.Context, itemID, userID, tenantID pgtype.UUID, bypassOwnership bool) error {
	if bypassOwnership {
		return nil
	}
	item, err := s.store.getComparisonItemByID(ctx, itemID, tenantID)
	if err != nil {
		return fmt.Errorf("getting comparison item for ownership check: %w", err)
	}
	return s.VerifyComparisonOwnership(ctx, item.ContentID, userID, tenantID, false)
}

// VerifyComparisonDimensionOwnership resolves the parent comparison from a
// dimension ID and then verifies ownership. Ref: §5.3 RBAC escape, BUG-003.
func (s *Service) VerifyComparisonDimensionOwnership(ctx context.Context, dimensionID, userID, tenantID pgtype.UUID, bypassOwnership bool) error {
	if bypassOwnership {
		return nil
	}
	dim, err := s.store.getComparisonDimensionByID(ctx, dimensionID, tenantID)
	if err != nil {
		return fmt.Errorf("getting comparison dimension for ownership check: %w", err)
	}
	return s.VerifyComparisonOwnership(ctx, dim.ContentID, userID, tenantID, false)
}

// DeleteComparisonType removes a comparison type and its cascaded children.
// It refuses to delete a type that is still referenced by comparison_items
// (Section 30 — in-use guard, BUG-006). System types (is_system=true) are
// also protected from deletion.
func (s *Service) DeleteComparisonType(ctx context.Context, id, tenantID pgtype.UUID) error {
	td, err := s.store.getComparisonTypeByID(ctx, id, tenantID)
	if err != nil {
		return fmt.Errorf("resolving comparison type: %w", err)
	}
	if td.IsSystem {
		return errors.New("system comparison types cannot be deleted")
	}
	count, err := s.store.countComparisonItemsByType(ctx, id, tenantID)
	if err != nil {
		return fmt.Errorf("checking comparison type usage: %w", err)
	}
	if count > 0 {
		return ErrComparisonTypeInUse
	}
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
// The type is first verified to belong to the caller's tenant so a guessed or
// leaked type_id UUID cannot read/write another tenant's access rows.
func (s *Service) SetComparisonTypeAccess(ctx context.Context, typeID, tenantID pgtype.UUID, input SetComparisonTypeAccessInput) error {
	if _, err := s.store.getComparisonTypeByID(ctx, typeID, tenantID); err != nil {
		return fmt.Errorf("verifying comparison type ownership: %w", err)
	}

	return s.store.setComparisonTypeAccess(ctx, gen.SetComparisonTypeAccessParams{
		TypeID:      typeID,
		UserGroupID: input.UserGroupID,
		CanCreate:   input.CanCreate,
		CanFill:     input.CanFill,
		AiCanFill:   input.AICanFill,
	})
}

// ListComparisonTypeAccess returns all access entries for a type. The type is
// first verified to belong to the caller's tenant (cross-tenant guard).
func (s *Service) ListComparisonTypeAccess(ctx context.Context, typeID, tenantID pgtype.UUID) ([]gen.ListComparisonTypeAccessRow, error) {
	if _, err := s.store.getComparisonTypeByID(ctx, typeID, tenantID); err != nil {
		return nil, fmt.Errorf("verifying comparison type ownership: %w", err)
	}

	return s.store.listComparisonTypeAccess(ctx, typeID, tenantID)
}

// DeleteComparisonTypeAccess removes an access entry for a user group.
func (s *Service) DeleteComparisonTypeAccess(ctx context.Context, typeID, tenantID, userGroupID pgtype.UUID) error {
	return s.store.deleteComparisonTypeAccess(ctx, typeID, tenantID, userGroupID)
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
func (s *Service) CreateComparisonItem(ctx context.Context, tenantID pgtype.UUID, input CreateComparisonItemInput) (gen.CreateComparisonItemRow, error) {
	if input.Title == "" {
		return gen.CreateComparisonItemRow{}, errors.New("title is required")
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
func (s *Service) ListComparisonItems(ctx context.Context, contentID, tenantID pgtype.UUID) ([]gen.ListComparisonItemsByContentRow, error) {
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
func (s *Service) UpdateComparisonItem(ctx context.Context, id, tenantID pgtype.UUID, input UpdateComparisonItemInput) (gen.UpdateComparisonItemRow, error) {
	if input.Title == "" {
		return gen.UpdateComparisonItemRow{}, errors.New("title is required")
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

// BatchReorderComparisonItems updates positions for all items in a comparison
// in a single pgx.Batch round-trip, avoiding N individual UPDATE queries
// (BUG-004). Each (id, position) pair is queued as a separate statement but
// sent to the database in one network round-trip.
func (s *Service) BatchReorderComparisonItems(ctx context.Context, contentID, tenantID pgtype.UUID, ids []pgtype.UUID, positions []int32) error {
	if len(ids) != len(positions) {
		return fmt.Errorf("ids and positions length mismatch")
	}
	if len(ids) == 0 {
		return nil
	}

	const reorderSQL = `UPDATE comparison_items SET position = $1
WHERE id = $2 AND content_id = $3 AND tenant_id = $4 AND deleted_at IS NULL`

	batch := &pgx.Batch{}
	for i, id := range ids {
		batch.Queue(reorderSQL, positions[i], id, contentID, tenantID)
	}

	br := s.db.Writer.SendBatch(ctx, batch)
	defer br.Close()

	for range ids {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("batch reorder item: %w", err)
		}
	}

	return nil
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
func (s *Service) CreateComparisonDimension(ctx context.Context, tenantID pgtype.UUID, input CreateComparisonDimensionInput) (gen.CreateComparisonDimensionRow, error) {
	if input.Label == "" {
		return gen.CreateComparisonDimensionRow{}, errors.New("label is required")
	}

	if !IsValidDimensionType(input.DimensionType) {
		return gen.CreateComparisonDimensionRow{}, fmt.Errorf("invalid dimension type: %s", input.DimensionType)
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
func (s *Service) ListComparisonDimensions(ctx context.Context, contentID, tenantID pgtype.UUID) ([]gen.ListComparisonDimensionsByContentRow, error) {
	return s.store.listComparisonDimensionsByContent(ctx, contentID, tenantID)
}

// UpdateComparisonDimensionInput holds parameters for updating a dimension.
type UpdateComparisonDimensionInput struct {
	Label         string
	DimensionType string
	Position      int32
}

// UpdateComparisonDimension updates an existing dimension.
func (s *Service) UpdateComparisonDimension(ctx context.Context, id, tenantID pgtype.UUID, input UpdateComparisonDimensionInput) (gen.UpdateComparisonDimensionRow, error) {
	if input.Label == "" {
		return gen.UpdateComparisonDimensionRow{}, errors.New("label is required")
	}

	if !IsValidDimensionType(input.DimensionType) {
		return gen.UpdateComparisonDimensionRow{}, fmt.Errorf("invalid dimension type: %s", input.DimensionType)
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

// BatchUpsertComparisonValues upserts multiple values for a single dimension in
// one round-trip using pgx.Batch (BUG-027). This replaces the per-value loop
// in the handler and avoids N round-trips for a large matrix.
func (s *Service) BatchUpsertComparisonValues(ctx context.Context, tenantID, dimensionID pgtype.UUID, inputs []UpsertComparisonValueInput) ([]gen.ComparisonValue, error) {
	if len(inputs) == 0 {
		return nil, nil
	}

	const upsertSQL = `INSERT INTO comparison_values (
    dimension_id, item_id, tenant_id, value, value_rating, value_boolean
) VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (dimension_id, item_id) DO UPDATE
SET value = EXCLUDED.value,
    value_rating = EXCLUDED.value_rating,
    value_boolean = EXCLUDED.value_boolean
RETURNING dimension_id, item_id, tenant_id, value, value_rating, value_boolean, created_at`

	batch := &pgx.Batch{}
	for _, input := range inputs {
		var value *string
		if input.Value != "" {
			value = &input.Value
		}
		var valueRating pgtype.Numeric
		if input.ValueRating != nil {
			if err := valueRating.Scan(fmt.Sprintf("%v", *input.ValueRating)); err != nil {
				return nil, fmt.Errorf("scanning value_rating: %w", err)
			}
		}
		batch.Queue(upsertSQL, dimensionID, input.ItemID, tenantID, value, valueRating, input.ValueBoolean)
	}

	results := make([]gen.ComparisonValue, 0, len(inputs))
	br := s.db.Writer.SendBatch(ctx, batch)
	defer br.Close()

	for range inputs {
		rows, err := br.Query()
		if err != nil {
			return nil, fmt.Errorf("batch upsert query: %w", err)
		}
		if !rows.Next() {
			rows.Close()
			return nil, fmt.Errorf("batch upsert returned no row")
		}
		var v gen.ComparisonValue
		if err := rows.Scan(
			&v.DimensionID, &v.ItemID, &v.TenantID, &v.Value,
			&v.ValueRating, &v.ValueBoolean, &v.CreatedAt,
		); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scanning batch upsert row: %w", err)
		}
		rows.Close()
		results = append(results, v)
	}

	return results, nil
}

// UpsertComparisonValueWithMetadata upserts a comparison value together with
// its provenance metadata JSONB (Section 103 — Comparison Engine Provenance).
// Used by the AI Comparison Engine to record the source
// (custom_field|rag|external|llm_knowledge), source_ref, confidence, and
// retrieved_at timestamp for each value it fills. The metadata column was
// added by migration 000373.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Provenance)
func (s *Service) UpsertComparisonValueWithMetadata(ctx context.Context, tenantID pgtype.UUID, input UpsertComparisonValueInput, metadata []byte) (gen.ComparisonValue, error) {
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

	var metadataBytes []byte
	if len(metadata) > 0 {
		metadataBytes = metadata
	}

	return s.store.upsertComparisonValueWithMetadata(ctx, gen.UpsertComparisonValueWithMetadataParams{
		DimensionID:  input.DimensionID,
		ItemID:       input.ItemID,
		TenantID:     tenantID,
		Value:        value,
		ValueRating:  valueRating,
		ValueBoolean: input.ValueBoolean,
		Metadata:     metadataBytes,
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
	Item       gen.ListComparisonItemsByContentRow `json:"item"`
	Dimensions []ComparisonMatrixDimension         `json:"dimensions"`
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

// ListComparisonsFilter holds optional filters for the comparison list.
type ListComparisonsFilter struct {
	TypeID  pgtype.UUID
	SpaceID pgtype.UUID
	Limit   int32
	Offset  int32
}

// ListComparisons returns a page of comparison content rows for the tenant,
// optionally filtered by type_id and space_id (Section 30, BUG-007/BUG-008).
// Pagination is offset-based per conventions/api-response-format.md.
func (s *Service) ListComparisons(ctx context.Context, tenantID pgtype.UUID, f ListComparisonsFilter) ([]gen.ListComparisonsFilteredRow, error) {
	if f.Limit <= 0 || f.Limit > MaxPageSize {
		f.Limit = DefaultPageSize
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	return s.store.listComparisonsFiltered(ctx, gen.ListComparisonsFilteredParams{
		TenantID:   tenantID,
		SpaceID:    f.SpaceID,
		TypeID:     f.TypeID,
		PageOffset: f.Offset,
		PageLimit:  f.Limit,
	})
}
