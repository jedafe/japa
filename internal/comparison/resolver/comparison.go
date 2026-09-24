package resolver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
)

// ComparisonResolver resolves published detailed comparisons so a comparison
// can be used as an item in another comparison (nested comparison items).
type ComparisonResolver struct {
	svc *content.Service
	q   *gen.Queries
}

// NewComparisonResolver creates a resolver backed by the content service and
// a query handle for the comparison search query.
func NewComparisonResolver(svc *content.Service, q *gen.Queries) Resolver {
	return &ComparisonResolver{svc: svc, q: q}
}

// Resolve fetches a content item by id or slug and verifies it is a comparison.
func (r *ComparisonResolver) Resolve(ctx context.Context, ref map[string]any) (ResolvedItem, bool, error) {
	if r == nil || r.svc == nil {
		return ResolvedItem{}, false, errors.New("comparison resolver not initialized")
	}

	idStr, _ := ref["id"].(string)
	slug, _ := ref["slug"].(string)

	if idStr == "" && slug == "" {
		return ResolvedItem{}, false, errors.New("comparison item_ref missing id or slug")
	}

	if idStr != "" {
		id, perr := ParseUUID(idStr)
		if perr != nil {
			return ResolvedItem{}, false, perr
		}

		row, err := r.svc.GetContent(ctx, id)
		if err != nil {
			if IsNotFound(err) {
				return ResolvedItem{}, false, nil
			}

			return ResolvedItem{}, false, fmt.Errorf("fetching comparison content: %w", err)
		}

		if row.ContentType != "comparison" {
			return ResolvedItem{}, false, nil
		}

		tags, err := r.contentTags(ctx, row.ID)
		if err != nil {
			return ResolvedItem{}, false, fmt.Errorf("fetching comparison content tags: %w", err)
		}

		return comparisonResolvedItem(row, pgtype.UUID{}, "", "", tags), true, nil
	}

	row, err := r.svc.GetContentBySlug(ctx, slug)
	if err != nil {
		if IsNotFound(err) {
			return ResolvedItem{}, false, nil
		}

		return ResolvedItem{}, false, fmt.Errorf("fetching comparison content: %w", err)
	}

	if row.ContentType != "comparison" {
		return ResolvedItem{}, false, nil
	}

	tags, err := r.contentTags(ctx, row.ID)
	if err != nil {
		return ResolvedItem{}, false, fmt.Errorf("fetching comparison content tags: %w", err)
	}

	return comparisonResolvedItem(row, pgtype.UUID{}, "", "", tags), true, nil
}

func (r *ComparisonResolver) contentTags(ctx context.Context, contentID pgtype.UUID) ([]string, error) {
	tagRows, err := r.svc.ListContentTags(ctx, contentID)
	if err != nil {
		return nil, err
	}

	tags := make([]string, 0, len(tagRows))

	for _, t := range tagRows {
		if t.Slug != "" {
			tags = append(tags, t.Slug)
		} else if t.Name != "" {
			tags = append(tags, t.Name)
		}
	}

	return tags, nil
}

// Search lists published detailed comparisons. The filters map may contain
// "nested_type" (a single comparison type slug) or "nested_types" (comma
// separated slugs) to restrict results.
func (r *ComparisonResolver) Search(ctx context.Context, query string, filters map[string]any, limit, offset int32) ([]ResolvedItem, error) {
	if r == nil || r.q == nil {
		return nil, errors.New("comparison resolver not initialized")
	}

	if limit <= 0 {
		limit = defaultSearchLimit
	}

	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	if offset < 0 {
		offset = 0
	}

	var typeSlugs []string

	if filters != nil {
		if v, ok := filters["nested_type"].(string); ok && v != "" {
			typeSlugs = []string{v}
		} else if v, ok := filters["nested_types"].(string); ok && v != "" {
			for _, p := range strings.Split(v, ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					typeSlugs = append(typeSlugs, p)
				}
			}
		}
	}

	rows, err := r.q.SearchComparisonContents(ctx, gen.SearchComparisonContentsParams{

		Q:          query,
		TypeSlugs:  typeSlugs,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("searching comparison contents: %w", err)
	}

	out := make([]ResolvedItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, searchComparisonRowToItem(row))
	}

	return out, nil
}

func searchComparisonRowToItem(row gen.SearchComparisonContentsRow) ResolvedItem {
	typeID, promotedFrom := "", ""
	if row.TypeID.Valid {
		typeID = row.TypeID.String()
	}

	if row.PromotedFromID.Valid {
		promotedFrom = row.PromotedFromID.String()
	}

	meta := map[string]any{
		"slug":          row.Slug,
		"content_type":  "comparison",
		"status":        row.Status,
		"visibility":    row.Visibility,
		"type_id":       typeID,
		"type_slug":     row.TypeSlug,
		"type_name":     row.TypeName,
		"promoted_from": promotedFrom,
		"score_inputs":  nestedComparisonScoreInputs(row.TypeSlug),
	}
	if row.GeoSpaceID.Valid {
		meta["geo_space_id"] = row.GeoSpaceID.String()
	}

	country := deref(row.TargetCountry)
	if country == "" {
		country = deref(row.SourceCountry)
	}

	if country != "" {
		meta["country"] = country
	}

	return ResolvedItem{
		Type:        "comparison",
		ID:          row.ID.String(),
		Title:       row.Title,
		Description: Excerpt(row.Body, 160),
		Metadata:    meta,
	}
}

func comparisonResolvedItem(row any, typeID pgtype.UUID, typeSlug, typeName string, tags []string) ResolvedItem {
	var id pgtype.UUID

	var title, slug, body, status, visibility string

	var geoSpaceID pgtype.UUID

	var sourceCountry, targetCountry *string

	var fieldValues []byte

	switch r := row.(type) {
	case gen.GetContentByIDRow:
		id, title, slug, body, status, visibility, geoSpaceID, sourceCountry, targetCountry, fieldValues = r.ID, r.Title, r.Slug, r.Body, r.Status, r.Visibility, r.GeoSpaceID, r.SourceCountry, r.TargetCountry, r.FieldValues
	case gen.GetContentBySlugRow:
		id, title, slug, body, status, visibility, geoSpaceID, sourceCountry, targetCountry, fieldValues = r.ID, r.Title, r.Slug, r.Body, r.Status, r.Visibility, r.GeoSpaceID, r.SourceCountry, r.TargetCountry, r.FieldValues
	default:
		return ResolvedItem{Type: "comparison"}
	}

	meta := map[string]any{
		"slug":         slug,
		"content_type": "comparison",
		"status":       status,
		"visibility":   visibility,
		"type_id":      nullUUID(typeID),
		"type_slug":    typeSlug,
		"type_name":    typeName,
		"score_inputs": nestedComparisonScoreInputs(typeSlug),
	}
	if geoSpaceID.Valid {
		meta["geo_space_id"] = geoSpaceID.String()
	}

	country := deref(targetCountry)
	if country == "" {
		country = deref(sourceCountry)
	}

	if country != "" {
		meta["country"] = country
	}

	if len(tags) > 0 {
		meta["tags"] = tags
	}

	// §30 Sub-spec 5: expose the backing content's field_values JSONB map so
	// matrix cells can hydrate from entity field values.
	if fields := parseDBFieldValues(fieldValues); len(fields) > 0 {
		meta["field_values"] = fields
	}

	return ResolvedItem{
		Type:        "comparison",
		ID:          id.String(),
		Title:       title,
		Description: Excerpt(body, 160),
		Metadata:    meta,
	}
}

func nestedComparisonScoreInputs(typeSlug string) map[string]any {
	var slug any
	if typeSlug != "" {
		slug = typeSlug
	}

	return map[string]any{
		"type_slug":        slug,
		"child_japa_score": nil,
		"freshness_score":  nil,
	}
}

func nullUUID(u pgtype.UUID) any {
	if u.Valid {
		return u.String()
	}

	return nil
}
