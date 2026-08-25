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
func (r *ComparisonResolver) Resolve(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, bool, error) {
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
		row, err := r.svc.GetContent(ctx, id, tenantID)
		if err != nil {
			if IsNotFound(err) {
				return ResolvedItem{}, false, nil
			}
			return ResolvedItem{}, false, fmt.Errorf("fetching comparison content: %w", err)
		}
		if row.ContentType != "comparison" {
			return ResolvedItem{}, false, nil
		}
		return comparisonResolvedItem(row.ID, row.Title, row.Slug, row.Body, row.Status, row.Visibility, pgtype.UUID{}, "", ""), true, nil
	}

	row, err := r.svc.GetContentBySlug(ctx, slug, tenantID)
	if err != nil {
		if IsNotFound(err) {
			return ResolvedItem{}, false, nil
		}
		return ResolvedItem{}, false, fmt.Errorf("fetching comparison content: %w", err)
	}
	if row.ContentType != "comparison" {
		return ResolvedItem{}, false, nil
	}
	return comparisonResolvedItem(row.ID, row.Title, row.Slug, row.Body, row.Status, row.Visibility, pgtype.UUID{}, "", ""), true, nil
}

// Search lists published detailed comparisons. The filters map may contain
// "nested_type" (a single comparison type slug) or "nested_types" (comma
// separated slugs) to restrict results.
func (r *ComparisonResolver) Search(ctx context.Context, query string, filters map[string]any, tenantID pgtype.UUID, limit int32) ([]ResolvedItem, error) {
	if r == nil || r.q == nil {
		return nil, errors.New("comparison resolver not initialized")
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
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
		TenantID:   tenantID,
		Q:          query,
		TypeSlugs:  typeSlugs,
		PageLimit:  limit,
		PageOffset: 0,
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
	return ResolvedItem{
		Type:        "comparison",
		ID:          row.ID.String(),
		Title:       row.Title,
		Description: Excerpt(row.Body, 160),
		Metadata: map[string]any{
			"slug":           row.Slug,
			"content_type":   "comparison",
			"status":         row.Status,
			"visibility":     row.Visibility,
			"type_id":        typeID,
			"type_slug":      row.TypeSlug,
			"type_name":      row.TypeName,
			"promoted_from":  promotedFrom,
		},
	}
}

func comparisonResolvedItem(id pgtype.UUID, title, slug, body, status, visibility string, typeID pgtype.UUID, typeSlug, typeName string) ResolvedItem {
	return ResolvedItem{
		Type:        "comparison",
		ID:          id.String(),
		Title:       title,
		Description: Excerpt(body, 160),
		Metadata: map[string]any{
			"slug":         slug,
			"content_type": "comparison",
			"status":       status,
			"visibility":   visibility,
			"type_id":      nullUUID(typeID),
			"type_slug":    typeSlug,
			"type_name":    typeName,
		},
	}
}

func nullUUID(u pgtype.UUID) any {
	if u.Valid {
		return u.String()
	}
	return nil
}
