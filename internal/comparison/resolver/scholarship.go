package resolver

import (
	"context"
	"errors"
	"fmt"
	"time"

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

func (r *scholarshipResolver) Resolve(ctx context.Context, ref map[string]any) (ResolvedItem, bool, error) {
	idStr := firstString(ref, "guide_id", "id")
	if idStr == "" {
		return ResolvedItem{}, false, errors.New("scholarship item_ref missing id/guide_id")
	}

	id, err := ParseUUID(idStr)
	if err != nil {
		return ResolvedItem{}, false, err
	}

	s, err := r.svc.GetScholarshipDetail(ctx, id)
	if err != nil {
		if IsNotFound(err) {
			return ResolvedItem{}, false, nil
		}

		return ResolvedItem{}, false, fmt.Errorf("fetching scholarship: %w", err)
	}

	return scholarshipToResolvedItem(s), true, nil
}

func (r *scholarshipResolver) Search(ctx context.Context, query string, _ map[string]any, limit, offset int32) ([]ResolvedItem, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	if offset < 0 {
		offset = 0
	}

	var resultLimit int
	if limit <= 0 {
		resultLimit = int(defaultSearchLimit)
	} else if limit > MaxSearchLimit {
		resultLimit = int(MaxSearchLimit)
	} else {
		resultLimit = int(limit)
	}

	page := int(offset/int32(resultLimit)) + 1 //nolint:gosec // G115: bounded

	f := scholarships.ScholarshipFilters{
		Q:     query,
		Page:  page,
		Limit: resultLimit,
	}

	results, _, err := r.svc.FindScholarships(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("searching scholarships: %w", err)
	}

	out := make([]ResolvedItem, 0, len(results))
	for _, s := range results {
		out = append(out, scholarshipToResolvedItem(s))
	}

	return out, nil
}

func scholarshipToResolvedItem(s scholarships.ScholarshipResult) ResolvedItem {
	meta := map[string]any{
		"slug":         s.Slug,
		"country":      s.Country,
		"institution":  s.Institution,
		"score_inputs": scholarshipScoreInputs(s),
	}
	if s.CountrySpaceID != "" {
		meta["geo_space_id"] = s.CountrySpaceID
	}

	if len(s.FieldsOfStudy) > 0 {
		meta["tags"] = s.FieldsOfStudy
	}

	return ResolvedItem{
		Type:        "scholarship",
		ID:          s.GuideID,
		Title:       s.Title,
		Description: Excerpt(s.Summary, 160),
		Metadata:    meta,
	}
}

func scholarshipScoreInputs(s scholarships.ScholarshipResult) map[string]any {
	// Scholarships do not have a "cost" in the same sense as tuition or living
	// expenses. The award amount is exposed as a separate `award` factor so
	// admins can configure a higher-is-better funding factor. The legacy `cost`
	// key is intentionally left missing here to avoid treating a scholarship as
	// an expense.
	var award any
	if s.AwardAmountUSD != nil {
		award = *s.AwardAmountUSD
	}

	var deadlineDays any

	if s.ApplicationDeadline != "" {
		if d, ok := parseScholarshipDeadline(s.ApplicationDeadline); ok {
			deadlineDays = int(d)
		}
	}

	fields := s.FieldsOfStudy
	if fields == nil {
		fields = []string{}
	}

	var country, institution, level any
	if s.Country != "" {
		country = s.Country
	}

	if s.Institution != "" {
		institution = s.Institution
	}

	if s.LevelOfStudy != "" {
		level = s.LevelOfStudy
	}

	return map[string]any{
		"cost":            nil,
		"award":           award,
		"deadline_days":   deadlineDays,
		"fields_of_study": fields,
		"country":         country,
		"institution":     institution,
		"level":           level,
		"language":        []string{},
	}
}

func parseScholarshipDeadline(deadline string) (float64, bool) {
	layouts := []string{
		time.RFC3339,
		"2006-01-02",
		"01/02/2006",
		"02/01/2006",
	}
	for _, layout := range layouts {
		if d, err := time.Parse(layout, deadline); err == nil {
			return time.Until(d).Hours() / 24, true
		}
	}

	return 0, false
}
