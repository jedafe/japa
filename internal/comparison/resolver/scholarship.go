package resolver

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/scholarships"
)

// scholarshipResolver resolves scholarship guides.
type scholarshipResolver struct {
	svc *scholarships.Service
}

// NewScholarshipResolver creates a resolver for scholarships.
func NewScholarshipResolver(svc *scholarships.Service) Resolver {
	return &scholarshipResolver{svc: svc}
}

func firstString(ref map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := ref[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func (r *scholarshipResolver) Resolve(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, bool, error) {
	idStr := firstString(ref, "guide_id", "id")
	if idStr == "" {
		return ResolvedItem{}, false, errors.New("scholarship item_ref missing id/guide_id")
	}

	id, err := ParseUUID(idStr)
	if err != nil {
		return ResolvedItem{}, false, err
	}

	s, err := r.svc.GetScholarshipDetail(ctx, tenantID, id)
	if err != nil {
		if IsNotFound(err) {
			return ResolvedItem{}, false, nil
		}
		return ResolvedItem{}, false, fmt.Errorf("fetching scholarship: %w", err)
	}

	return ResolvedItem{
		Type:        "scholarship",
		ID:          s.GuideID,
		Title:       s.Title,
		Description: Excerpt(s.Summary, 160),
		Metadata: map[string]any{
			"slug":        s.Slug,
			"country":     s.Country,
			"institution": s.Institution,
		},
	}, true, nil
}

func (r *scholarshipResolver) Search(ctx context.Context, query string, _ map[string]any, tenantID pgtype.UUID, limit int32) ([]ResolvedItem, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var page int
	var resultLimit int
	if limit <= 0 {
		resultLimit = 20
	} else if limit > 100 {
		resultLimit = 100
	} else {
		resultLimit = int(limit)
	}

	f := scholarships.ScholarshipFilters{
		Q:     query,
		Page:  page,
		Limit: resultLimit,
	}

	results, _, err := r.svc.FindScholarships(ctx, tenantID, f)
	if err != nil {
		return nil, fmt.Errorf("searching scholarships: %w", err)
	}

	out := make([]ResolvedItem, 0, len(results))
	for _, s := range results {
		out = append(out, ResolvedItem{
			Type:        "scholarship",
			ID:          s.GuideID,
			Title:       s.Title,
			Description: Excerpt(s.Summary, 160),
			Metadata: map[string]any{
				"slug":        s.Slug,
				"country":     s.Country,
				"institution": s.Institution,
			},
		})
	}
	return out, nil
}
