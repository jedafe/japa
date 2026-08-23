package resolver

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/languagetesthub"
)

// languageTestResolver resolves language tests (IELTS, TOEFL, etc).
type languageTestResolver struct {
	svc *languagetesthub.Service
}

// NewLanguageTestResolver creates a resolver for language tests.
func NewLanguageTestResolver(svc *languagetesthub.Service) Resolver {
	return &languageTestResolver{svc: svc}
}

func (r *languageTestResolver) Resolve(ctx context.Context, ref map[string]any, _ pgtype.UUID) (ResolvedItem, bool, error) {
	if slug, ok := ref["slug"].(string); ok && slug != "" {
		t, err := r.svc.GetTestBySlug(ctx, slug)
		if err != nil {
			if IsNotFound(err) {
				return ResolvedItem{}, false, nil
			}
			return ResolvedItem{}, false, fmt.Errorf("fetching language test by slug: %w", err)
		}
		return languageTestToResolvedItem(t), true, nil
	}

	if idStr, ok := ref["id"].(string); ok && idStr != "" {
		id, err := ParseUUID(idStr)
		if err != nil {
			return ResolvedItem{}, false, err
		}
		t, err := r.svc.GetTestByID(ctx, id)
		if err != nil {
			if IsNotFound(err) {
				return ResolvedItem{}, false, nil
			}
			return ResolvedItem{}, false, fmt.Errorf("fetching language test by id: %w", err)
		}
		return languageTestToResolvedItem(t), true, nil
	}

	return ResolvedItem{}, false, errors.New("language_test item_ref missing id or slug")
}

func (r *languageTestResolver) Search(ctx context.Context, query string, _ map[string]any, tenantID pgtype.UUID, limit int32) ([]ResolvedItem, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	tests, err := r.svc.SearchTests(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("searching language tests: %w", err)
	}

	out := make([]ResolvedItem, 0, len(tests))
	for _, t := range tests {
		out = append(out, languageTestToResolvedItem(t))
	}
	return out, nil
}

func languageTestToResolvedItem(t languagetesthub.TestDTO) ResolvedItem {
	return ResolvedItem{
		Type:        "languagetest",
		ID:          t.ID.String(),
		Title:       t.Name,
		Description: "",
		URL:         t.OfficialURL,
		Metadata: map[string]any{
			"slug": t.Slug,
		},
	}
}
