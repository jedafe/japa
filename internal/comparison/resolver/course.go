package resolver

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/university_courses"
)

// courseResolver resolves university course offerings for comparison.
type courseResolver struct {
	svc *university_courses.Service
}

// NewCourseResolver creates a resolver for university course offerings.
func NewCourseResolver(svc *university_courses.Service) Resolver {
	return &courseResolver{svc: svc}
}

func (r *courseResolver) Resolve(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, bool, error) {
	idStr, _ := ref["id"].(string)
	slug, _ := ref["slug"].(string)

	if idStr == "" && slug == "" {
		return ResolvedItem{}, false, errors.New("course item_ref missing id or slug")
	}

	var offering university_courses.CourseOffering
	var err error
	if idStr != "" {
		id, perr := ParseUUID(idStr)
		if perr != nil {
			return ResolvedItem{}, false, perr
		}
		offering, err = r.svc.GetOfferingByID(ctx, tenantID, id)
	} else {
		offering, err = r.svc.GetOfferingBySlug(ctx, tenantID, slug)
	}

	if err != nil {
		if IsNotFound(err) {
			return ResolvedItem{}, false, nil
		}
		return ResolvedItem{}, false, fmt.Errorf("fetching course offering: %w", err)
	}

	return offeringToResolvedItem(offering), true, nil
}

func (r *courseResolver) Search(ctx context.Context, query string, filters map[string]any, tenantID pgtype.UUID, limit int32, offset int32) ([]ResolvedItem, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}
	if offset < 0 {
		offset = 0
	}

	universityID := parseUUIDFilter(filters, "university_id")
	if !universityID.Valid {
		return nil, errors.New("university_id filter is required for course search")
	}

	offerings, err := r.svc.SearchOfferings(ctx, tenantID, universityID, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("searching course offerings: %w", err)
	}
	out := make([]ResolvedItem, 0, len(offerings))
	for _, o := range offerings {
		out = append(out, offeringToResolvedItem(o))
	}
	return out, nil
}

func pgNumericToFloat64(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}
	f, err := n.Float64Value()
	if err != nil {
		return 0
	}
	return f.Float64
}

func offeringToResolvedItem(o university_courses.CourseOffering) ResolvedItem {
	title := o.CourseTitle
	if o.UniversityName != "" {
		title = fmt.Sprintf("%s — %s", o.CourseTitle, o.UniversityName)
	}

	cost := pgNumericToFloat64(o.Tuition)
	var durationWeeks any
	if o.DurationYears.Valid {
		years := pgNumericToFloat64(o.DurationYears)
		if years > 0 {
			durationWeeks = int(years * 52)
		}
	} else if o.DefaultDurationYears.Valid {
		years := pgNumericToFloat64(o.DefaultDurationYears)
		if years > 0 {
			durationWeeks = int(years * 52)
		}
	}

	scoreInputs := map[string]any{
		"cost":           cost,
		"duration_weeks": durationWeeks,
		"language":       []string{o.Language},
		"field":          deref(o.FieldOfStudy),
		"degree_type":    deref(o.DegreeType),
		"university_id":  o.UniversityID.String(),
	}

	return ResolvedItem{
		Type:        "course",
		ID:          o.ID.String(),
		Title:       title,
		Description: Excerpt(deref(o.CourseDescription), 160),
		Metadata: map[string]any{
			"slug":            o.Slug,
			"course_slug":     o.CourseSlug,
			"university_id":   o.UniversityID.String(),
			"university_name": o.UniversityName,
			"university_slug": o.UniversitySlug,
			"degree_type":     deref(o.DegreeType),
			"score_inputs":    scoreInputs,
		},
	}
}
