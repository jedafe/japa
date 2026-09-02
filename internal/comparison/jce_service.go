package comparison

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/comparison/resolver"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
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
	Visibility string
	Status     string
	SpaceID    pgtype.UUID
	TypeID     pgtype.UUID
	Depth      string // 'quick' or 'detailed'
	Items      []JCEItemInput
	Dimensions []string
	SmartMode  string // 'none' | 'eligibility' | 'cost' | 'pathway'
	Config     []byte // JSON metadata config (e.g. profile_id)
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
	TypeSlug   string
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
	Sort       string
	Limit      int32
	Offset     int32
}

// JCEComparisonListResult is the paged result of ListJCEComparisons.
type JCEComparisonListResult struct {
	Items           []gen.ListComparisonsWithFiltersRow
	Total           int32
	Page            int32
	Size            int32
	SortOptions     []string `json:"sort_options,omitempty"`
	PageSizeOptions []int32  `json:"page_size_options,omitempty"`
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
	// Detailed comparisons may be public and indexable once published.
	// Callers may override visibility/status for detailed comparisons only.
	visibility := input.Visibility
	if visibility == "" {
		visibility = content.VisibilityPrivate
	}
	status := input.Status
	if status == "" {
		status = content.StatusDraft
	}
	if depth == DepthQuick {
		visibility = content.VisibilityPrivate
		status = content.StatusDraft
	}

	var seoIndexable bool
	if depth == DepthDetailed {
		seoIndexable = visibility == content.VisibilityPublic && status == content.StatusPublished
	}

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

	smartMode := input.SmartMode
	if smartMode == "" {
		smartMode = SmartModeNone
	}
	config := input.Config
	if len(config) == 0 {
		config = []byte("{}")
	}
	metadata := gen.ComparisonContentMetadatum{
		ContentID:      contentRow.ID,
		TenantID:       tenantID,
		Depth:          depth,
		PromotedFromID: pgtype.UUID{Valid: false},
		PromotedAt:     pgtype.Timestamptz{Valid: false},
		SmartMode:      &smartMode,
		ScoreVersion:   1,
		Config:         config,
	}

	if err := q.InsertComparisonContentMetadata(ctx, gen.InsertComparisonContentMetadataParams{
		ContentID:      contentRow.ID,
		TenantID:       tenantID,
		Depth:          depth,
		PromotedFromID: metadata.PromotedFromID,
		PromotedAt:     metadata.PromotedAt,
		SmartMode:      &smartMode,
		ScoreVersion:   metadata.ScoreVersion,
		Config:         config,
	}); err != nil {
		return gen.CreateContentRow{}, gen.ComparisonContentMetadatum{}, fmt.Errorf("inserting comparison metadata: %w", err)
	}

	return contentRow, metadata, nil
}

// jceTypeConfig mirrors the JSONB config stored on comparison_type_definitions.
type jceTypeConfig struct {
	ItemRefTypes           []string `json:"item_ref_types"`
	DefaultItemRefType     string   `json:"default_item_ref_type"`
	AllowNestedComparisons bool     `json:"allow_nested_comparisons"`
	AllowedNestedTypes     []string `json:"allowed_nested_types"`
	MaxItemCount           int      `json:"max_item_count"`
	DefaultItemCount       int      `json:"default_item_count"`
	ScopeChain             []string `json:"scope_chain"`
	ContentType            string   `json:"content_type"`
}

func parseJCETypeConfig(b []byte) (jceTypeConfig, error) {
	var cfg jceTypeConfig
	if len(b) == 0 || string(b) == "{}" {
		return cfg, nil
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func sliceContains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// CreateJCEComparison creates a quick or detailed JCE comparison.
func (s *Service) CreateJCEComparison(ctx context.Context, tenantID, userID pgtype.UUID, input CreateJCEComparisonInput) (JCEComparisonResult, error) {
	var typeDef *gen.GetComparisonTypeByIDRow
	var typeCfg jceTypeConfig
	if input.TypeID.Valid {
		td, err := s.store.getComparisonTypeByID(ctx, input.TypeID, tenantID)
		if err != nil {
			return JCEComparisonResult{}, fmt.Errorf("resolving comparison type: %w", err)
		}
		typeDef = &td

		cfg, cerr := parseJCETypeConfig(typeDef.Config)
		if cerr != nil {
			return JCEComparisonResult{}, fmt.Errorf("parsing comparison type config: %w", cerr)
		}
		typeCfg = cfg

		allowed, aerr := s.canCreateComparisonForType(ctx, input.TypeID, userID, tenantID)
		if aerr != nil {
			return JCEComparisonResult{}, fmt.Errorf("checking comparison type access: %w", aerr)
		}
		if !allowed {
			return JCEComparisonResult{}, errpkg.ErrForbidden
		}
	}

	if typeCfg.MaxItemCount > 0 && len(input.Items) > typeCfg.MaxItemCount {
		return JCEComparisonResult{}, fmt.Errorf("%w: too many items (max %d)", errpkg.ErrInvalid, typeCfg.MaxItemCount)
	}

	var autoFields []gen.ListCustomFieldsByGroupRow
	if len(input.Dimensions) == 0 && typeDef != nil && typeDef.FieldGroupID.Valid {
		fields, ferr := s.cfSvc.ListFieldsByGroup(ctx, typeDef.FieldGroupID, tenantID)
		if ferr != nil {
			return JCEComparisonResult{}, fmt.Errorf("listing field-group fields: %w", ferr)
		}
		autoFields = fields
	}

	// Collapse duplicate inputs before persisting so the store never inserts
	// multiple rows for the same logical item or dimension.
	input.Items = dedupeJCEItemInputs(input.Items)
	input.Dimensions = dedupeStringDimensions(input.Dimensions)

	return db.WithTxResult(ctx, s.db.Writer, func(q *gen.Queries) (JCEComparisonResult, error) {
		contentRow, metadata, err := s.createJCEContentAndMetadata(ctx, q, tenantID, userID, input)
		if err != nil {
			return JCEComparisonResult{}, err
		}

		for i, item := range input.Items {
			resolved, err := s.resolveItemRef(ctx, item, tenantID)
			if err != nil {
				return JCEComparisonResult{}, fmt.Errorf("resolving item ref %d: %w", i, err)
			}

			// Validate item_ref against the comparison type configuration.
			if resolved != nil {
				refType := resolved.Type
				if input.TypeID.Valid && len(typeCfg.ItemRefTypes) > 0 && !sliceContains(typeCfg.ItemRefTypes, refType) {
					return JCEComparisonResult{}, fmt.Errorf("%w: item_ref type %q is not allowed for this comparison type", errpkg.ErrInvalid, refType)
				}
				if refType == "comparison" {
					if !typeCfg.AllowNestedComparisons {
						return JCEComparisonResult{}, fmt.Errorf("%w: nested comparisons are not enabled for this type", errpkg.ErrInvalid)
					}
					if len(typeCfg.AllowedNestedTypes) > 0 {
						nestedSlug, _ := resolved.Metadata["type_slug"].(string)
						if !sliceContains(typeCfg.AllowedNestedTypes, nestedSlug) {
							return JCEComparisonResult{}, fmt.Errorf("%w: nested comparison type %q is not allowed", errpkg.ErrInvalid, nestedSlug)
						}
					}
				}
			}

			title := item.Title
			if title == "" && resolved != nil {
				title = resolved.Title
			}
			if title == "" {
				continue
			}

			desc := toNullableString(item.Description)
			if desc == nil && resolved != nil && resolved.Description != "" {
				desc = &resolved.Description
			}

			itemTypeID := item.TypeID
			if !itemTypeID.Valid && input.TypeID.Valid {
				itemTypeID = input.TypeID
			}

			_, ierr := q.CreateComparisonItem(ctx, gen.CreateComparisonItemParams{
				ContentID:   contentRow.ID,
				TenantID:    tenantID,
				TypeID:      itemTypeID,
				Title:       title,
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

// resolveItemRef attempts to resolve an item reference into a ResolvedItem.
// It returns (nil, nil) when the item has no resolvable ref.
func (s *Service) resolveItemRef(ctx context.Context, item JCEItemInput, tenantID pgtype.UUID) (*resolver.ResolvedItem, error) {
	if s.resolver == nil || len(item.ItemRef) == 0 {
		return nil, nil
	}

	ref, err := resolver.CoerceMap(item.ItemRef)
	if err != nil {
		return nil, err
	}
	if len(ref) == 0 {
		return nil, nil
	}

	resolved, err := s.resolver.Resolve(ctx, ref, tenantID)
	if err != nil {
		return nil, err
	}
	return &resolved, nil
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
	items, _ = dedupeComparisonItems(items)

	dims, err := s.store.listComparisonDimensionsByContent(ctx, contentRow.ID, tenantID)
	if err != nil {
		return JCEComparison{}, fmt.Errorf("listing comparison dimensions: %w", err)
	}
	dims, _ = dedupeComparisonDimensions(dims)

	cmp := JCEComparison{
		Content:    contentRow,
		Metadata:   metadata,
		Items:      items,
		Dimensions: dims,
	}
	for _, it := range items {
		if it.TypeID.Valid {
			typeRow, err := s.store.getComparisonTypeByID(ctx, it.TypeID, tenantID)
			if err == nil {
				cmp.TypeSlug = typeRow.Slug
			}
			break
		}
	}
	return cmp, nil
}

// ListJCEComparisons returns a filtered, paged list of comparisons with metadata.
func (s *Service) ListJCEComparisons(ctx context.Context, tenantID pgtype.UUID, f JCEComparisonFilter) (JCEComparisonListResult, error) {
	if f.Limit <= 0 || f.Limit > MaxPageSize {
		f.Limit = DefaultPageSize
	}
	if f.Offset < 0 {
		f.Offset = 0
	}

	var depth, status, visibility, query, sort *string
	if f.Depth != "" && f.Depth != "all" {
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
	if f.Sort != "" {
		sort = &f.Sort
	}

	mine := f.Mine
	userID := f.UserID
	if !mine {
		userID = pgtype.UUID{Valid: false}
	}

	items, err := s.store.listComparisonsWithFilters(ctx, gen.ListComparisonsWithFiltersParams{
		TenantID:   tenantID,
		SpaceID:    f.SpaceID,
		TypeID:     f.TypeID,
		Depth:      depth,
		Status:     status,
		Visibility: visibility,
		Mine:       &mine,
		UserID:     userID,
		Q:          query,
		Sort:       sort,
		PageLimit:  f.Limit,
		PageOffset: f.Offset,
	})
	if err != nil {
		return JCEComparisonListResult{}, err
	}

	total, err := s.store.countComparisonsWithFilters(ctx, gen.CountComparisonsWithFiltersParams{
		TenantID:   tenantID,
		SpaceID:    f.SpaceID,
		TypeID:     f.TypeID,
		Depth:      depth,
		Status:     status,
		Visibility: visibility,
		Mine:       &mine,
		UserID:     userID,
		Q:          query,
	})
	if err != nil {
		return JCEComparisonListResult{}, err
	}

	page := int32(1)
	if f.Limit > 0 {
		page = f.Offset/f.Limit + 1
	}

	sortOptions := []string{"created_at", "title", "popularity"}
	if s.settingsSvc != nil {
		raw := s.settingsSvc.GetStringOr(ctx, tenantID, "jce.comparisons.sort_options", `["created_at","title","popularity"]`)
		_ = json.Unmarshal([]byte(raw), &sortOptions)
	}

	pageSizeOptions := []int32{12, 24, 48, 96}
	if s.settingsSvc != nil {
		raw := s.settingsSvc.GetStringOr(ctx, tenantID, "jce.comparisons.page_size_options", `[12,24,48,96]`)
		_ = json.Unmarshal([]byte(raw), &pageSizeOptions)
	}

	return JCEComparisonListResult{
		Items:           items,
		Total:           total,
		Page:            page,
		Size:            f.Limit,
		SortOptions:     sortOptions,
		PageSizeOptions: pageSizeOptions,
	}, nil
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

	// Quick comparisons are never indexable; detailed ones are indexable only when public and published.
	indexable := metadata.Depth == DepthDetailed &&
		visibility == content.VisibilityPublic &&
		status == content.StatusPublished
	seoIndexable := &indexable
	_, uerr := s.contentSvc.UpdateContent(ctx, id, tenantID, userID, content.UpdateContentInput{
		Title:        title,
		Slug:         input.Slug,
		Body:         input.Body,
		Visibility:   visibility,
		Status:       status,
		SeoIndexable: seoIndexable,
	}, bypassOwnership)
	if uerr != nil {
		return JCEComparison{}, fmt.Errorf("updating comparison content: %w", uerr)
	}

	if len(input.Config) > 0 {
		cfg := input.Config
		if len(cfg) == 0 {
			cfg = metadata.Config
		}
		if err := s.store.updateComparisonContentMetadata(ctx, gen.UpdateComparisonContentMetadataParams{
			ContentID:    id,
			TenantID:     tenantID,
			Depth:        &metadata.Depth,
			SmartMode:    metadata.SmartMode,
			ScoreVersion: &metadata.ScoreVersion,
			Config:       cfg,
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

		// Collapse any duplicate source rows before cloning so promotion does
		// not spread existing data-integrity issues.
		dedupedItems, itemIDMap := dedupeComparisonItems(existing.Items)
		dedupedDimensions, dimIDMap := dedupeComparisonDimensions(existing.Dimensions)

		// Clone items.
		newItemMap := make(map[pgtype.UUID]pgtype.UUID, len(dedupedItems))
		for i, item := range dedupedItems {
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
			newItemMap[item.ID] = newItem.ID
		}

		// Clone dimensions.
		newDimMap := make(map[pgtype.UUID]pgtype.UUID, len(dedupedDimensions))
		for i, dim := range dedupedDimensions {
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
			newDimMap[dim.ID] = newDim.ID
		}

		// Clone values, mapping duplicate source IDs to the surviving row and
		// merging any values attached to removed duplicates.
		values, verr := q.GetComparisonValuesByContent(ctx, gen.GetComparisonValuesByContentParams{
			ContentID: id,
			TenantID:  tenantID,
		})
		if verr != nil {
			return JCEComparisonResult{}, fmt.Errorf("listing values for promotion: %w", verr)
		}

		mergedValues := make(map[pgtype.UUID]map[pgtype.UUID]gen.ComparisonValue)
		for _, v := range values {
			dedupedItemID, ok := itemIDMap[v.ItemID]
			if !ok {
				continue
			}
			dedupedDimID, ok := dimIDMap[v.DimensionID]
			if !ok {
				continue
			}
			newItemID, ok := newItemMap[dedupedItemID]
			if !ok {
				continue
			}
			newDimID, ok := newDimMap[dedupedDimID]
			if !ok {
				continue
			}
			if mergedValues[newItemID] == nil {
				mergedValues[newItemID] = make(map[pgtype.UUID]gen.ComparisonValue)
			}
			mergedValues[newItemID][newDimID] = mergeComparisonValues(mergedValues[newItemID][newDimID], v)
		}

		for newItemID, dimMap := range mergedValues {
			for newDimID, v := range dimMap {
				_, verr := q.UpsertComparisonValueWithMetadata(ctx, gen.UpsertComparisonValueWithMetadataParams{
					DimensionID:  newDimID,
					ItemID:       newItemID,
					TenantID:     tenantID,
					Value:        v.Value,
					ValueRating:  v.ValueRating,
					ValueBoolean: v.ValueBoolean,
					Metadata:     v.Metadata,
				})
				if verr != nil {
					return JCEComparisonResult{}, fmt.Errorf("cloning value: %w", verr)
				}
			}
		}

		// Link promotion metadata.
		promotedAt := pgtype.Timestamptz{Time: time.Now(), Valid: true}
		promotedFromID := existing.Content.ID
		if err := q.UpdateComparisonContentMetadata(ctx, gen.UpdateComparisonContentMetadataParams{
			ContentID:      newContent.ID,
			TenantID:       tenantID,
			Depth:          &newMetadata.Depth,
			PromotedFromID: promotedFromID,
			PromotedAt:     promotedAt,
			SmartMode:      newMetadata.SmartMode,
			ScoreVersion:   &newMetadata.ScoreVersion,
			Config:         newMetadata.Config,
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

// SearchJCEEntities returns candidate items of the given entity type and, when
// the resolver supports it, the total number of matching items.
func (s *Service) SearchJCEEntities(ctx context.Context, entityType, query string, filters map[string]any, tenantID pgtype.UUID, limit int32, offset int32) ([]resolver.ResolvedItem, int64, error) {
	if s.resolver == nil {
		return nil, -1, errors.New("resolver not configured")
	}

	g, ctx := errgroup.WithContext(ctx)

	var items []resolver.ResolvedItem
	g.Go(func() error {
		var err error
		items, err = s.resolver.Search(ctx, entityType, query, filters, tenantID, limit, offset)
		return err
	})

	var total int64 = -1
	g.Go(func() error {
		var err error
		total, err = s.resolver.Count(ctx, entityType, query, filters, tenantID)
		if err != nil {
			slog.ErrorContext(ctx, "jce entity count failed", "type", entityType, "error", err)
			total = -1
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, -1, err
	}
	return items, total, nil
}

// ResolveJCEEntity resolves a single entity reference by id or slug.
func (s *Service) ResolveJCEEntity(ctx context.Context, entityType, idOrSlug string, tenantID pgtype.UUID) (resolver.ResolvedItem, error) {
	if s.resolver == nil {
		return resolver.ResolvedItem{}, errors.New("resolver not configured")
	}
	ref := map[string]any{"type": entityType}
	if _, err := uuid.Parse(idOrSlug); err == nil {
		ref["id"] = idOrSlug
	} else {
		ref["slug"] = idOrSlug
	}
	return s.resolver.Resolve(ctx, ref, tenantID)
}

// GetJCEComparisonMatrix returns the assembled matrix with items, dimensions, and values.
func (s *Service) GetJCEComparisonMatrix(ctx context.Context, id, tenantID pgtype.UUID) ([]ComparisonMatrixItem, error) {
	return s.GetComparisonMatrix(ctx, id, tenantID)
}

// defaultSelectionTokenTTL is the fallback lifetime of a guest comparison selection token (15 minutes).
const defaultSelectionTokenTTL = 15 * time.Minute

// selectionTokenKey returns the Valkey key for a selection token.
func selectionTokenKey(token string) string {
	return "jc:compare:selection:" + token
}

// generateToken returns a cryptographically random 32-byte hex token.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// CreateSelectionTokenInput holds the guest selection payload.
type CreateSelectionTokenInput struct {
	TenantID pgtype.UUID
	TypeSlug string
	ItemRefs []map[string]any
}

// CreateSelectionToken stores a guest selection in Valkey and returns the token.
func (s *Service) CreateSelectionToken(ctx context.Context, input CreateSelectionTokenInput) (string, error) {
	if s.vk == nil {
		return "", errors.New("valkey not configured")
	}
	token, err := generateToken()
	if err != nil {
		return "", err
	}

	payload := map[string]any{
		"type_slug":  input.TypeSlug,
		"item_refs":  input.ItemRefs,
		"created_at": time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encoding selection token payload: %w", err)
	}

	ttl := defaultSelectionTokenTTL
	if s.settingsSvc != nil {
		ttl = time.Duration(s.settingsSvc.GetIntOr(ctx, input.TenantID, "jce.selection_token_ttl_minutes", 15)) * time.Minute
	}

	if err := s.vk.Set(ctx, selectionTokenKey(token), data, ttl).Err(); err != nil {
		return "", fmt.Errorf("storing selection token: %w", err)
	}
	return token, nil
}

// CreateJCEComparisonFromToken creates a quick comparison from a guest selection token.
func (s *Service) CreateJCEComparisonFromToken(ctx context.Context, tenantID, userID pgtype.UUID, token string, input CreateJCEComparisonInput) (JCEComparisonResult, error) {
	if s.vk == nil {
		return JCEComparisonResult{}, errors.New("valkey not configured")
	}

	data, err := s.vk.Get(ctx, selectionTokenKey(token)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return JCEComparisonResult{}, errors.New("selection token not found or expired")
		}
		return JCEComparisonResult{}, fmt.Errorf("fetching selection token: %w", err)
	}

	var payload struct {
		TypeSlug string           `json:"type_slug"`
		ItemRefs []map[string]any `json:"item_refs"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return JCEComparisonResult{}, fmt.Errorf("decoding selection token payload: %w", err)
	}

	items := make([]JCEItemInput, 0, len(payload.ItemRefs))
	for i, ref := range payload.ItemRefs {
		item := JCEItemInput{
			Position: toInt32(i),
			TypeID:   input.TypeID,
			ItemRef:  marshalJSONB(ref),
		}

		if s.resolver != nil && len(ref) > 0 {
			resolved, rerr := s.resolver.Resolve(ctx, ref, tenantID)
			if rerr == nil {
				item.Title = resolved.Title
				item.Description = resolved.Description
				if meta := resolved.Metadata; meta != nil {
					if v, ok := meta["image_id"].(string); ok && v != "" {
						if id, err := resolver.ParseUUID(v); err == nil {
							item.ImageID = id
						}
					}
					if v, ok := meta["geo_space_id"].(string); ok && v != "" {
						if id, err := resolver.ParseUUID(v); err == nil {
							item.GeoSpaceID = id
						}
					}
				}
			}
		}

		if item.Title == "" {
			if t, ok := ref["title"].(string); ok {
				item.Title = t
			}
		}
		if item.Description == "" {
			if d, ok := ref["description"].(string); ok {
				item.Description = d
			}
		}

		items = append(items, item)
	}

	input.Items = items
	if input.Depth == "" {
		input.Depth = DepthQuick
	}

	if input.Title == "" {
		titles := make([]string, 0, len(items))
		for _, it := range items {
			if it.Title != "" {
				titles = append(titles, it.Title)
			}
		}
		switch len(titles) {
		case 0:
			input.Title = "Quick comparison"
		case 1:
			input.Title = fmt.Sprintf("Comparison of %s", titles[0])
		case 2:
			input.Title = fmt.Sprintf("Comparison of %s and %s", titles[0], titles[1])
		default:
			input.Title = fmt.Sprintf("Comparison of %s and %d others", titles[0], len(titles)-1)
		}
	}

	result, err := s.CreateJCEComparison(ctx, tenantID, userID, input)
	if err != nil {
		return JCEComparisonResult{}, fmt.Errorf("creating comparison from token: %w", err)
	}

	// Best-effort token deletion. The key has a TTL, so a failed deletion is not fatal.
	_ = s.vk.Del(ctx, selectionTokenKey(token)).Err()

	return result, nil
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

// jceComparisonResponse builds an API-friendly response map from a JCE comparison.
// It decodes JSONB item_ref fields so they are returned as JSON objects rather
// than base64-encoded byte arrays.
func jceComparisonResponse(cmp JCEComparison) map[string]any {
	items := make([]map[string]any, 0, len(cmp.Items))
	for _, it := range cmp.Items {
		items = append(items, jceComparisonItemResponse(it))
	}
	res := map[string]any{
		"content":    cmp.Content,
		"metadata":   cmp.Metadata,
		"items":      items,
		"dimensions": cmp.Dimensions,
	}
	if cmp.TypeSlug != "" {
		res["type_slug"] = cmp.TypeSlug
	}
	return res
}

func jceComparisonItemResponse(it gen.ListComparisonItemsByContentRow) map[string]any {
	item := map[string]any{
		"id":       it.ID.String(),
		"title":    it.Title,
		"position": it.Position,
	}
	if it.TypeID.Valid {
		item["type_id"] = it.TypeID.String()
	}
	if it.ImageID.Valid {
		item["image_id"] = it.ImageID.String()
	}
	if it.GeoSpaceID.Valid {
		item["geo_space_id"] = it.GeoSpaceID.String()
	}
	if it.Description != nil && *it.Description != "" {
		item["description"] = *it.Description
	}
	if len(it.ItemRef) > 0 {
		var ref map[string]any
		if err := json.Unmarshal(it.ItemRef, &ref); err == nil {
			item["item_ref"] = ref
		}
	}
	return item
}

// jceMatrixResponse builds an API-friendly matrix response where each row is a
// flat item with its dimensions.
func jceMatrixResponse(matrix []ComparisonMatrixItem) []map[string]any {
	out := make([]map[string]any, 0, len(matrix))
	for _, row := range matrix {
		it := row.Item
		item := map[string]any{
			"id":         it.ID.String(),
			"title":      it.Title,
			"position":   it.Position,
			"dimensions": row.Dimensions,
		}
		if it.Description != nil && *it.Description != "" {
			item["description"] = *it.Description
		}
		out = append(out, item)
	}
	return out
}
