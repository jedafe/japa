package resolver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/content/visibility"
	"github.com/jedafe/japa/internal/db/gen"
)

// ContentResolver resolves content items as comparison references.
type ContentResolver struct {
	svc *content.Service
}

// NewContentResolver creates a resolver backed by the content service.
func NewContentResolver(svc *content.Service) *ContentResolver {
	return &ContentResolver{svc: svc}
}

// Resolve fetches a content item by id or slug.
func (r *ContentResolver) Resolve(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, bool, error) {
	if r == nil || r.svc == nil {
		return ResolvedItem{}, false, errors.New("content resolver not initialized")
	}

	if idStr, ok := ref["id"].(string); ok && idStr != "" {
		id, err := ParseUUID(idStr)
		if err != nil {
			return ResolvedItem{}, false, err
		}
		row, err := r.svc.GetContent(ctx, id, tenantID)
		if err != nil {
			if IsNotFound(err) {
				return ResolvedItem{}, false, nil
			}
			return ResolvedItem{}, false, fmt.Errorf("fetching content by id: %w", err)
		}
		tags, err := r.contentTags(ctx, row.ID)
		if err != nil {
			return ResolvedItem{}, false, fmt.Errorf("fetching content tags: %w", err)
		}
		return contentItemFromAny(row, tags), true, nil
	}

	if slug, ok := ref["slug"].(string); ok && slug != "" {
		row, err := r.svc.GetContentBySlug(ctx, slug, tenantID)
		if err != nil {
			if IsNotFound(err) {
				return ResolvedItem{}, false, nil
			}
			return ResolvedItem{}, false, fmt.Errorf("fetching content by slug: %w", err)
		}
		tags, err := r.contentTags(ctx, row.ID)
		if err != nil {
			return ResolvedItem{}, false, fmt.Errorf("fetching content tags: %w", err)
		}
		return contentItemFromAny(row, tags), true, nil
	}

	return ResolvedItem{}, false, errors.New("content item_ref missing id or slug")
}

func (r *ContentResolver) contentTags(ctx context.Context, contentID pgtype.UUID) ([]string, error) {
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

// Search lists public content matching the query.
func (r *ContentResolver) Search(ctx context.Context, query string, filters map[string]any, tenantID pgtype.UUID, limit int32) ([]ResolvedItem, error) {
	if r == nil || r.svc == nil {
		return nil, errors.New("content resolver not initialized")
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	contentType := ""
	if filters != nil {
		if v, ok := filters["content_type"].(string); ok {
			contentType = v
		}
	}

	filter := visibility.VisibilityFilter{}

	var rows []any
	emptyCursor := pgtype.Timestamptz{}

	if contentType != "" {
		items, lerr := r.svc.ListContentsByType(ctx, tenantID, contentType, emptyCursor, limit, filter)
		if lerr != nil {
			return nil, fmt.Errorf("listing contents by type: %w", lerr)
		}
		for _, it := range items {
			if query == "" || containsQuery(it.Title, it.Slug, it.Body, query) {
				rows = append(rows, it)
			}
		}
	} else {
		items, lerr := r.svc.ListContents(ctx, tenantID, query, emptyCursor, limit, filter)
		if lerr != nil {
			return nil, fmt.Errorf("listing contents: %w", lerr)
		}
		for _, it := range items {
			rows = append(rows, it)
		}
	}

	return mapContentRows(rows, limit), nil
}

func containsQuery(title, slug, body, query string) bool {
	q := lower(query)
	return contains(lower(title), q) || contains(lower(slug), q) || contains(lower(body), q)
}

func lower(s string) string          { return strings.ToLower(s) }
func contains(s, substr string) bool { return strings.Contains(s, substr) }

func mapContentRows(rows []any, limit int32) []ResolvedItem {
	out := make([]ResolvedItem, 0, len(rows))
	for _, row := range rows {
		item := contentItemFromAny(row, nil)
		if item.ID == "" {
			continue
		}
		out = append(out, item)
		if int32(len(out)) >= limit {
			break
		}
	}
	return out
}

func contentItemFromAny(row any, tags []string) ResolvedItem {
	var id pgtype.UUID
	var title, body, contentType, slug, status, visibility string
	var geoSpaceID pgtype.UUID
	var sourceCountry, targetCountry *string

	switch r := row.(type) {
	case gen.GetContentByIDRow:
		id, title, body, contentType, slug, status, visibility, geoSpaceID, sourceCountry, targetCountry = r.ID, r.Title, r.Body, r.ContentType, r.Slug, r.Status, r.Visibility, r.GeoSpaceID, r.SourceCountry, r.TargetCountry
	case gen.GetContentBySlugRow:
		id, title, body, contentType, slug, status, visibility, geoSpaceID, sourceCountry, targetCountry = r.ID, r.Title, r.Body, r.ContentType, r.Slug, r.Status, r.Visibility, r.GeoSpaceID, r.SourceCountry, r.TargetCountry
	case gen.ListContentsByTenantRow:
		id, title, body, contentType, slug, status, visibility, geoSpaceID, sourceCountry, targetCountry = r.ID, r.Title, r.Body, r.ContentType, r.Slug, r.Status, r.Visibility, r.GeoSpaceID, r.SourceCountry, r.TargetCountry
	case gen.ListContentsByTypeRow:
		id, title, body, contentType, slug, status, visibility, geoSpaceID, sourceCountry, targetCountry = r.ID, r.Title, r.Body, r.ContentType, r.Slug, r.Status, r.Visibility, r.GeoSpaceID, r.SourceCountry, r.TargetCountry
	default:
		return ResolvedItem{}
	}

	meta := map[string]any{
		"content_type": contentType,
		"slug":         slug,
		"status":       status,
		"visibility":   visibility,
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

	return ResolvedItem{
		ID:          id.String(),
		Title:       title,
		Description: Excerpt(body, 160),
		Metadata:    meta,
	}
}
