package comparison

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Depth values for comparison_content_metadata.
const (
	DepthQuick    = "quick"
	DepthDetailed = "detailed"
)

// Smart mode values.
const (
	SmartModeNone        = "none"
	SmartModeEligibility = "eligibility"
	SmartModeCost        = "cost"
	SmartModePathway     = "pathway"
	SmartModeAIAdvisor   = "ai_advisor"
)

// JCEItemInput is an item passed by the JCE comparison picker.
type JCEItemInput struct {
	Title       string
	Description string
	TypeID      pgtype.UUID
	ImageID     pgtype.UUID
	GeoSpaceID  pgtype.UUID
	Position    int32
	ItemRef     []byte
}

// CreateJCEComparisonInput holds parameters for creating a quick or detailed comparison.
type CreateJCEComparisonInput struct {
	Title      string
	Body       string
	SpaceID    pgtype.UUID
	TypeID     pgtype.UUID
	Depth      string // 'quick' or 'detailed'
	Items      []JCEItemInput
	Dimensions []string
}

// JCEComparisonResult holds the result of creating a JCE comparison.
type JCEComparisonResult struct {
	ID         pgtype.UUID `json:"id"`
	TenantID   pgtype.UUID `json:"tenant_id"`
	AuthorID   pgtype.UUID `json:"author_id"`
	Title      string      `json:"title"`
	Slug       string      `json:"slug"`
	Visibility string      `json:"visibility"`
	Status     string      `json:"status"`
	Depth      string      `json:"depth"`
	SpaceID    pgtype.UUID `json:"space_id"`
}

// JCEComparison is the assembled detail view for the JCE API.
type JCEComparison struct {
	Content    gen.GetComparisonBySlugOrIDRow
	Metadata   gen.ComparisonContentMetadatum
	Items      []gen.ListComparisonItemsByContentRow
	Dimensions []gen.ListComparisonDimensionsByContentRow
}

// JCEComparisonFilter holds listing filters.
type JCEComparisonFilter struct {
	SpaceID    pgtype.UUID
	TypeID     pgtype.UUID
	Depth      string
	Status     string
	Visibility string
	Mine       bool
	UserID     pgtype.UUID
	Query      string
	Limit      int32
	Offset     int32
}

// createJCEContentAndMetadata creates the content row and the metadata row inside a transaction.
func (s *Service) createJCEContentAndMetadata(
	ctx context.Context,
	q *gen.Queries,
	tenantID, userID pgtype.UUID,
	input CreateJCEComparisonInput,
) (gen.CreateContentRow, gen.ComparisonContentMetadatum, error) {
	if input.Title == "" {
		return gen.CreateContentRow{}, gen.ComparisonContentMetadatum{}, errors.New("title is required")
	}

	depth := input.Depth
	if depth == "" {
		depth = DepthQuick
	}
	if depth != DepthQuick && depth != DepthDetailed {
		return gen.CreateContentRow{}, gen.ComparisonContentMetadatum{}, errors.New("invalid comparison depth")
	}

	// Quick comparisons are always private, draft, and non-indexable.
	// Detailed comparisons start as drafts so the user can fill values before publishing.
	visibility := content.VisibilityPrivate
	status := content.StatusDraft
	seoIndexable := false

	contentRow, err := s.contentSvc.CreateContentWithQueries(ctx, q, tenantID, content.CreateContentInput{
		AuthorID:     userID,
		SpaceID:      input.SpaceID,
		ContentType:  content.TypeComparison,
		Title:        input.Title,
		Body:         input.Body,
		SeoIndexable: seoIndexable,
		Status:       status,
		Visibility:   visibility,
	})
	if err != nil {
		return gen.CreateContentRow{}, gen.ComparisonContentMetadatum{}, fmt.Errorf("creating comparison content: %w", err)
	}

	smartMode := SmartModeNone
	metadata := gen.ComparisonContentMetadatum{
		ContentID:      contentRow.ID,
		TenantID:       tenantID,
		Depth:          depth,
		PromotedFromID: pgtype.UUID{Valid: false},
		PromotedAt:     pgtype.Timestamptz{Valid: false},
		SmartMode:      &smartMode,
		ScoreVersion:   1,
		Config:         []byte("{}"),
	}

	if err := q.InsertComparisonContentMetadata(ctx, gen.InsertComparisonContentMetadataParams{
		ContentID:      contentRow.ID,
		TenantID:       tenantID,
		Depth:          depth,
		PromotedFromID: metadata.PromotedFromID,
		PromotedAt:     metadata.PromotedAt,
		SmartMode:      &smartMode,
		ScoreVersion:   metadata.ScoreVersion,
		Config:         metadata.Config,
	}); err != nil {
		return gen.CreateContentRow{}, gen.ComparisonContentMetadatum{}, fmt.Errorf("inserting comparison metadata: %w", err)
	}

	return contentRow, metadata, nil
}

// CreateJCEComparison creates a quick or detailed JCE comparison.
func (s *Service) CreateJCEComparison(ctx context.Context, tenantID, userID pgtype.UUID, input CreateJCEComparisonInput) (JCEComparisonResult, error) {
	var typeDef *gen.GetComparisonTypeByIDRow
	if input.TypeID.Valid {
		td, err := s.store.getComparisonTypeByID(ctx, input.TypeID, tenantID)
		if err != nil {
			return JCEComparisonResult{}, fmt.Errorf("resolving comparison type: %w", err)
		}
		typeDef = &td

		allowed, aerr := s.canCreateComparisonForType(ctx, input.TypeID, userID, tenantID)
		if aerr != nil {
			return JCEComparisonResult{}, fmt.Errorf("checking comparison type access: %w", aerr)
		}
		if !allowed {
			return JCEComparisonResult{}, errpkg.ErrForbidden
		}
	}

	var autoFields []gen.ListCustomFieldsByGroupRow
	if len(input.Dimensions) == 0 && typeDef != nil && typeDef.FieldGroupID.Valid {
		fields, ferr := s.cfSvc.ListFieldsByGroup(ctx, typeDef.FieldGroupID, tenantID)
		if ferr != nil {
			return JCEComparisonResult{}, fmt.Errorf("listing field-group fields: %w", ferr)
		}
		autoFields = fields
	}

	return db.WithTxResult(ctx, s.db.Writer, func(q *gen.Queries) (JCEComparisonResult, error) {
		contentRow, metadata, err := s.createJCEContentAndMetadata(ctx, q, tenantID, userID, input)
		if err != nil {
			return JCEComparisonResult{}, err
		}

		for i, item := range input.Items {
			if item.Title == "" {
				continue
			}

			desc := toNullableString(item.Description)
			_, ierr := q.CreateComparisonItem(ctx, gen.CreateComparisonItemParams{
				ContentID:   contentRow.ID,
				TenantID:    tenantID,
				TypeID:      item.TypeID,
				Title:       item.Title,
				Description: desc,
				ImageID:     item.ImageID,
				GeoSpaceID:  item.GeoSpaceID,
				Position:    toInt32(i),
				ItemRef:     item.ItemRef,
			})
			if ierr != nil {
				return JCEComparisonResult{}, fmt.Errorf("creating comparison item %d: %w", i, ierr)
			}
		}

		if autoFields != nil {
			for i, f := range autoFields {
				_, derr := q.CreateComparisonDimension(ctx, gen.CreateComparisonDimensionParams{
					ContentID:     contentRow.ID,
					TenantID:      tenantID,
					TypeID:        input.TypeID,
					Label:         f.Label,
					DimensionType: fieldTypeToDimensionType(f.FieldType),
					Position:      toInt32(i),
				})
				if derr != nil {
					return JCEComparisonResult{}, fmt.Errorf("auto-creating dimension %d: %w", i, derr)
				}
			}
		} else {
			for i, label := range input.Dimensions {
				if label == "" {
					continue
				}
				_, derr := q.CreateComparisonDimension(ctx, gen.CreateComparisonDimensionParams{
					ContentID:     contentRow.ID,
					TenantID:      tenantID,
					TypeID:        input.TypeID,
					Label:         label,
					DimensionType: DimensionTypeText,
					Position:      toInt32(i),
				})
				if derr != nil {
					return JCEComparisonResult{}, fmt.Errorf("creating comparison dimension %d: %w", i, derr)
				}
			}
		}

		return JCEComparisonResult{
			ID:         contentRow.ID,
			TenantID:   contentRow.TenantID,
			AuthorID:   contentRow.AuthorID,
			Title:      contentRow.Title,
			Slug:       contentRow.Slug,
			Visibility: contentRow.Visibility,
			Status:     contentRow.Status,
			Depth:      metadata.Depth,
			SpaceID:    contentRow.SpaceID,
		}, nil
	})
}

// GetJCEComparison returns a comparison plus metadata.
// Callers must enforce visibility before returning to users.
func (s *Service) GetJCEComparison(ctx context.Context, idOrSlug string, tenantID pgtype.UUID) (JCEComparison, error) {
	var id pgtype.UUID
	var slug *string

	if parsed, err := uuid.Parse(idOrSlug); err == nil {
		id = pgtype.UUID{Bytes: parsed, Valid: true}
	} else {
		slug = &idOrSlug
	}

	contentRow, err := s.store.getComparisonBySlugOrID(ctx, id, slug, tenantID)
	if err != nil {
		return JCEComparison{}, fmt.Errorf("getting comparison: %w", err)
	}

	metadata, err := s.store.getComparisonContentMetadata(ctx, contentRow.ID, tenantID)
	if err != nil {
		return JCEComparison{}, fmt.Errorf("getting comparison metadata: %w", err)
	}

	items, err := s.store.listComparisonItemsByContent(ctx, contentRow.ID, tenantID)
	if err != nil {
		return JCEComparison{}, fmt.Errorf("listing comparison items: %w", err)
	}

	dims, err := s.store.listComparisonDimensionsByContent(ctx, contentRow.ID, tenantID)
	if err != nil {
		return JCEComparison{}, fmt.Errorf("listing comparison dimensions: %w", err)
	}

	return JCEComparison{
		Content:    contentRow,
		Metadata:   metadata,
		Items:      items,
		Dimensions: dims,
	}, nil
}

// ListJCEComparisons returns a filtered list of comparisons with metadata.
func (s *Service) ListJCEComparisons(ctx context.Context, tenantID pgtype.UUID, f JCEComparisonFilter) ([]gen.ListComparisonsWithFiltersRow, error) {
	if f.Limit <= 0 || f.Limit > MaxPageSize {
		f.Limit = DefaultPageSize
	}
	if f.Offset < 0 {
		f.Offset = 0
	}

	var depth, status, visibility, query *string
	if f.Depth != "" {
		depth = &f.Depth
	}
	if f.Status != "" {
		status = &f.Status
	}
	if f.Visibility != "" {
		visibility = &f.Visibility
	}
	if f.Query != "" {
		query = &f.Query
	}

	mine := f.Mine
	userID := f.UserID
	if !mine {
		userID = pgtype.UUID{Valid: false}
	}

	return s.store.listComparisonsWithFilters(ctx, gen.ListComparisonsWithFiltersParams{
		TenantID:   tenantID,
		SpaceID:    f.SpaceID,
		TypeID:     f.TypeID,
		Depth:      depth,
		Status:     status,
		Visibility: visibility,
		Mine:       &mine,
		UserID:     userID,
		Q:          query,
		PageLimit:  f.Limit,
		PageOffset: f.Offset,
	})
}

// UpdateJCEComparisonInput holds editable fields for a quick or detailed comparison.
type UpdateJCEComparisonInput struct {
	Title      string
	Body       string
	Slug       string
	Visibility string
	Status     string
	Config     []byte
}

// UpdateJCEComparison updates the content and metadata of a JCE comparison.
// Only the author or users with comparison.edit_any may update.
func (s *Service) UpdateJCEComparison(ctx context.Context, id, tenantID, userID pgtype.UUID, input UpdateJCEComparisonInput, bypassOwnership bool) (JCEComparison, error) {
	existing, err := s.contentSvc.GetContent(ctx, id, tenantID)
	if err != nil {
		return JCEComparison{}, fmt.Errorf("getting comparison for update: %w", err)
	}

	if !bypassOwnership && existing.AuthorID.Valid && existing.AuthorID.Bytes != userID.Bytes {
		return JCEComparison{}, errpkg.ErrForbidden
	}

	metadata, err := s.store.getComparisonContentMetadata(ctx, id, tenantID)
	if err != nil {
		return JCEComparison{}, fmt.Errorf("getting comparison metadata: %w", err)
	}

	title := input.Title
	if title == "" {
		title = existing.Title
	}

	visibility := input.Visibility
	status := input.Status
	if visibility == "" {
		visibility = existing.Visibility
	}
	if status == "" {
		status = existing.Status
	}

	// Quick comparisons cannot be made public or published.
	if metadata.Depth == DepthQuick {
		visibility = content.VisibilityPrivate
		status = content.StatusDraft
	}

	seoIndexable := false
	_, uerr := s.contentSvc.UpdateContent(ctx, id, tenantID, userID, content.UpdateContentInput{
		Title:        title,
		Slug:         input.Slug,
		Body:         input.Body,
		Visibility:   visibility,
		Status:       status,
		SeoIndexable: &seoIndexable,
	}, bypassOwnership)
	if uerr != nil {
		return JCEComparison{}, fmt.Errorf("updating comparison content: %w", uerr)
	}

	if len(input.Config) > 0 {
		if err := s.store.updateComparisonContentMetadata(ctx, gen.UpdateComparisonContentMetadataParams{
			ContentID: id,
			TenantID:  tenantID,
			Config:    input.Config,
		}); err != nil {
			return JCEComparison{}, fmt.Errorf("updating comparison metadata: %w", err)
		}
	}

	return s.GetJCEComparison(ctx, id.String(), tenantID)
}

// DeleteJCEComparison soft-deletes a JCE comparison.
func (s *Service) DeleteJCEComparison(ctx context.Context, id, tenantID, userID pgtype.UUID, bypassOwnership bool) error {
	existing, err := s.contentSvc.GetContent(ctx, id, tenantID)
	if err != nil {
		return fmt.Errorf("getting comparison for delete: %w", err)
	}

	if !bypassOwnership && existing.AuthorID.Valid && existing.AuthorID.Bytes != userID.Bytes {
		return errpkg.ErrForbidden
	}

	if err := s.contentSvc.DeleteContent(ctx, id, tenantID); err != nil {
		return fmt.Errorf("deleting comparison content: %w", err)
	}

	return nil
}

// PromoteJCEComparison promotes a quick comparison to a new detailed draft.
func (s *Service) PromoteJCEComparison(ctx context.Context, id, tenantID, userID pgtype.UUID, bypassOwnership bool) (JCEComparisonResult, error) {
	existing, err := s.GetJCEComparison(ctx, id.String(), tenantID)
	if err != nil {
		return JCEComparisonResult{}, fmt.Errorf("getting comparison for promotion: %w", err)
	}

	if existing.Metadata.Depth != DepthQuick {
		return JCEComparisonResult{}, errors.New("only quick comparisons can be promoted")
	}

	if !bypassOwnership && existing.Content.AuthorID.Valid && existing.Content.AuthorID.Bytes != userID.Bytes {
		return JCEComparisonResult{}, errpkg.ErrForbidden
	}

	// Resolve comparison type from the first item, if any.
	typeID := pgtype.UUID{Valid: false}
	if len(existing.Items) > 0 && existing.Items[0].TypeID.Valid {
		typeID = existing.Items[0].TypeID
	}

	return db.WithTxResult(ctx, s.db.Writer, func(q *gen.Queries) (JCEComparisonResult, error) {
		newContent, newMetadata, err := s.createJCEContentAndMetadata(ctx, q, tenantID, userID, CreateJCEComparisonInput{
			Title:   existing.Content.Title,
			Body:    existing.Content.Body,
			SpaceID: existing.Content.SpaceID,
			TypeID:  typeID,
			Depth:   DepthDetailed,
		})
		if err != nil {
			return JCEComparisonResult{}, fmt.Errorf("creating detailed comparison: %w", err)
		}

		// Clone items.
		itemMap := make(map[pgtype.UUID]pgtype.UUID, len(existing.Items))
		for i, item := range existing.Items {
			newItem, ierr := q.CreateComparisonItem(ctx, gen.CreateComparisonItemParams{
				ContentID:   newContent.ID,
				TenantID:    tenantID,
				TypeID:      item.TypeID,
				Title:       item.Title,
				Description: item.Description,
				ImageID:     item.ImageID,
				GeoSpaceID:  item.GeoSpaceID,
				Position:    item.Position,
				ItemRef:     item.ItemRef,
			})
			if ierr != nil {
				return JCEComparisonResult{}, fmt.Errorf("cloning item %d: %w", i, ierr)
			}
			itemMap[item.ID] = newItem.ID
		}

		// Clone dimensions.
		dimMap := make(map[pgtype.UUID]pgtype.UUID, len(existing.Dimensions))
		for i, dim := range existing.Dimensions {
			newDim, derr := q.CreateComparisonDimension(ctx, gen.CreateComparisonDimensionParams{
				ContentID:     newContent.ID,
				TenantID:      tenantID,
				TypeID:        dim.TypeID,
				Label:         dim.Label,
				DimensionType: dim.DimensionType,
				Position:      dim.Position,
			})
			if derr != nil {
				return JCEComparisonResult{}, fmt.Errorf("cloning dimension %d: %w", i, derr)
			}
			dimMap[dim.ID] = newDim.ID
		}

		// Clone values.
		values, verr := q.GetComparisonValuesByContent(ctx, gen.GetComparisonValuesByContentParams{
			ContentID: id,
			TenantID:  tenantID,
		})
		if verr != nil {
			return JCEComparisonResult{}, fmt.Errorf("listing values for promotion: %w", verr)
		}

		for _, v := range values {
			newItemID, ok := itemMap[v.ItemID]
			if !ok {
				continue
			}
			newDimID, ok := dimMap[v.DimensionID]
			if !ok {
				continue
			}

			_, verr := q.UpsertComparisonValue(ctx, gen.UpsertComparisonValueParams{
				DimensionID:  newDimID,
				ItemID:       newItemID,
				TenantID:     tenantID,
				Value:        v.Value,
				ValueRating:  v.ValueRating,
				ValueBoolean: v.ValueBoolean,
			})
			if verr != nil {
				return JCEComparisonResult{}, fmt.Errorf("cloning value: %w", verr)
			}
		}

		// Link promotion metadata.
		promotedAt := pgtype.Timestamptz{Time: time.Now(), Valid: true}
		promotedFromID := existing.Content.ID
		if err := q.UpdateComparisonContentMetadata(ctx, gen.UpdateComparisonContentMetadataParams{
			ContentID:      newContent.ID,
			TenantID:       tenantID,
			PromotedFromID: promotedFromID,
			PromotedAt:     promotedAt,
		}); err != nil {
			return JCEComparisonResult{}, fmt.Errorf("updating promotion metadata: %w", err)
		}

		return JCEComparisonResult{
			ID:         newContent.ID,
			TenantID:   newContent.TenantID,
			AuthorID:   newContent.AuthorID,
			Title:      newContent.Title,
			Slug:       newContent.Slug,
			Visibility: newContent.Visibility,
			Status:     newContent.Status,
			Depth:      newMetadata.Depth,
			SpaceID:    newContent.SpaceID,
		}, nil
	})
}

// GetJCEComparisonMatrix returns the assembled matrix with items, dimensions, and values.
func (s *Service) GetJCEComparisonMatrix(ctx context.Context, id, tenantID pgtype.UUID) ([]ComparisonMatrixItem, error) {
	return s.GetComparisonMatrix(ctx, id, tenantID)
}

// toNullableString returns a pointer for non-empty strings.
func toNullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// marshalJSONB returns a JSONB-safe []byte for an empty object or the given value.
func marshalJSONB(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}
