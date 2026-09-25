package resolver

import (
	"context"
	"errors"
	"fmt"

	"github.com/jedafe/japa/internal/languagetesthub"
)

// languageTestResolver resolves language tests (IELTS, TOEFL, etc).
type languageTestResolver struct {
	svc *languagetesthub.Service
}

// NewLanguageTestResolver creates a resolver for language tests. The service
// projects custom_field_values into TestDTO.FieldValues on every read (§80
// Tool Surface Hydration), so the resolver consumes that projection — cfv
// flows without a second query.
func NewLanguageTestResolver(svc *languagetesthub.Service, opts ...Option) Resolver {
	newResolverOptions(opts...)

	return &languageTestResolver{svc: svc}
}

func (r *languageTestResolver) Resolve(ctx context.Context, ref map[string]any) (ResolvedItem, bool, error) {
	if slug, ok := ref["slug"].(string); ok && slug != "" {
		t, err := r.svc.GetTestBySlug(ctx, slug)
		if err != nil {
			if IsNotFound(err) {
				return ResolvedItem{}, false, nil
			}

			return ResolvedItem{}, false, fmt.Errorf("fetching language test by slug: %w", err)
		}

		return languageTestToResolvedItem(t, t.FieldValues), true, nil
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

		return languageTestToResolvedItem(t, t.FieldValues), true, nil
	}

	return ResolvedItem{}, false, errors.New("language_test item_ref missing id or slug")
}

func (r *languageTestResolver) Search(ctx context.Context, query string, _ map[string]any, limit, offset int32) ([]ResolvedItem, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	if offset < 0 {
		offset = 0
	}

	tests, err := r.svc.SearchTests(ctx, query, limit+offset)
	if err != nil {
		return nil, fmt.Errorf("searching language tests: %w", err)
	}

	if int(offset) >= len(tests) {
		return []ResolvedItem{}, nil
	}

	end := int(offset) + int(limit)
	if end > len(tests) {
		end = len(tests)
	}

	page := tests[offset:end]

	out := make([]ResolvedItem, 0, len(page))
	for _, t := range page {
		out = append(out, languageTestToResolvedItem(t, t.FieldValues))
	}

	return out, nil
}

func languageTestToResolvedItem(t languagetesthub.TestDTO, cfv map[string]any) ResolvedItem {
	var validity any
	if t.ValidityMonths > 0 {
		validity = t.ValidityMonths
	}

	meta := map[string]any{
		"slug": t.Slug,
		"score_inputs": map[string]any{
			// Typed columns stay authoritative; cfv fills gaps (§80).
			"cost":            cfvValue(cfv, "fee_usd", "cost", "fee"),
			"speed":           cfvValue(cfv, "result_days", "score_delivery_days"),
			"validity_months": validity,
			"accepted_by":     []string{},
			"score_required":  cfvValue(cfv, "score_required", "minimum_score"),
			"language":        []string{},
		},
	}

	// §80 Tool Surface Hydration: language_tests has no JSONB field bag, so
	// community field values project as field_values for matrix hydration.
	if len(cfv) > 0 {
		meta["field_values"] = cfv
	}

	return ResolvedItem{
		Type:        "languagetest",
		ID:          t.ID.String(),
		Title:       t.Name,
		Description: "",
		URL:         t.OfficialURL,
		Metadata:    meta,
	}
}

// cfvValue returns the first present value for the given field keys.
func cfvValue(cfv map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := cfv[k]; ok && v != nil {
			return v
		}
	}

	return nil
}
