package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/content/visibility"
	"github.com/jedafe/japa/internal/db/gen"
)

// JobListingResolver resolves public job listings (contents with
// content_type='job_listing') into JCE comparison references.
type JobListingResolver struct {
	svc *content.Service
}

// NewJobListingResolver creates a resolver backed by the content service.
func NewJobListingResolver(svc *content.Service) *JobListingResolver {
	return &JobListingResolver{svc: svc}
}

// Resolve fetches a job listing by id or slug.
func (r *JobListingResolver) Resolve(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, bool, error) {
	if r == nil || r.svc == nil {
		return ResolvedItem{}, false, errors.New("job_listing resolver not initialized")
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
			return ResolvedItem{}, false, fmt.Errorf("fetching job listing by id: %w", err)
		}
		if !isJobListing(row.ContentType, row.Status, row.Visibility) {
			return ResolvedItem{}, false, nil
		}
		tags, err := r.contentTags(ctx, row.ID)
		if err != nil {
			return ResolvedItem{}, false, fmt.Errorf("fetching job listing tags: %w", err)
		}
		return jobListingFromAny(row, tags), true, nil
	}

	if slug, ok := ref["slug"].(string); ok && slug != "" {
		row, err := r.svc.GetContentBySlug(ctx, slug, tenantID)
		if err != nil {
			if IsNotFound(err) {
				return ResolvedItem{}, false, nil
			}
			return ResolvedItem{}, false, fmt.Errorf("fetching job listing by slug: %w", err)
		}
		if !isJobListing(row.ContentType, row.Status, row.Visibility) {
			return ResolvedItem{}, false, nil
		}
		tags, err := r.contentTags(ctx, row.ID)
		if err != nil {
			return ResolvedItem{}, false, fmt.Errorf("fetching job listing tags: %w", err)
		}
		return jobListingFromAny(row, tags), true, nil
	}

	return ResolvedItem{}, false, errors.New("job_listing item_ref missing id or slug")
}

func (r *JobListingResolver) contentTags(ctx context.Context, contentID pgtype.UUID) ([]string, error) {
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

// Search lists published, public job listings matching the query.
func (r *JobListingResolver) Search(ctx context.Context, query string, filters map[string]any, tenantID pgtype.UUID, limit int32) ([]ResolvedItem, error) {
	if r == nil || r.svc == nil {
		return nil, errors.New("job_listing resolver not initialized")
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	emptyCursor := pgtype.Timestamptz{}
	items, err := r.svc.ListContentsByType(ctx, tenantID, "job_listing", emptyCursor, limit, visibility.VisibilityFilter{})
	if err != nil {
		return nil, fmt.Errorf("listing job listings: %w", err)
	}

	out := make([]ResolvedItem, 0, len(items))
	for _, it := range items {
		if query != "" && !containsQuery(it.Title, it.Slug, it.Body, query) {
			continue
		}
		out = append(out, jobListingFromAny(it, nil))
		if int32(len(out)) >= limit {
			break
		}
	}
	return out, nil
}

func isJobListing(contentType, status, visibility string) bool {
	return contentType == "job_listing" && status == "published" && visibility == "public"
}

func jobListingFromAny(row any, tags []string) ResolvedItem {
	var id pgtype.UUID
	var title, body, contentType, slug, status, visibility string
	var geoSpaceID pgtype.UUID
	var sourceCountry, targetCountry *string
	var fieldValues []byte

	switch r := row.(type) {
	case gen.GetContentByIDRow:
		id, title, body, contentType, slug, status, visibility, geoSpaceID, sourceCountry, targetCountry, fieldValues = r.ID, r.Title, r.Body, r.ContentType, r.Slug, r.Status, r.Visibility, r.GeoSpaceID, r.SourceCountry, r.TargetCountry, r.FieldValues
	case gen.GetContentBySlugRow:
		id, title, body, contentType, slug, status, visibility, geoSpaceID, sourceCountry, targetCountry, fieldValues = r.ID, r.Title, r.Body, r.ContentType, r.Slug, r.Status, r.Visibility, r.GeoSpaceID, r.SourceCountry, r.TargetCountry, r.FieldValues
	case gen.ListContentsByTypeRow:
		id, title, body, contentType, slug, status, visibility, geoSpaceID, sourceCountry, targetCountry, fieldValues = r.ID, r.Title, r.Body, r.ContentType, r.Slug, r.Status, r.Visibility, r.GeoSpaceID, r.SourceCountry, r.TargetCountry, r.FieldValues
	default:
		return ResolvedItem{}
	}

	meta := map[string]any{
		"content_type": contentType,
		"slug":         slug,
		"status":       status,
		"visibility":   visibility,
		"score_inputs": jobListingScoreInputs(fieldValues, geoSpaceID),
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
		Type:        "job_listing",
		ID:          id.String(),
		Title:       title,
		Description: Excerpt(body, 160),
		Metadata:    meta,
	}
}

func jobListingScoreInputs(fieldValues []byte, geoSpaceID pgtype.UUID) map[string]any {
	fields := map[string]any{}
	if len(fieldValues) > 0 {
		_ = json.Unmarshal(fieldValues, &fields)
	}

	var income any
	if v, ok := fields["salary_max"]; ok && v != nil && !isEmptyFieldValue(v) {
		income = v
	} else if v, ok := fields["salary_min"]; ok && v != nil && !isEmptyFieldValue(v) {
		income = v
	}

	var contractType any
	if v, ok := fields["job_type"]; ok && v != nil && !isEmptyFieldValue(v) {
		contractType = v
	}

	var sponsorship, remote any
	if v, ok := fields["visa_sponsorship"]; ok && v != nil {
		sponsorship = jobListingTruthy(v)
	}
	if v, ok := fields["location_type"]; ok && v != nil {
		s := fmt.Sprint(v)
		switch strings.ToLower(s) {
		case "remote", "hybrid":
			remote = true
		case "onsite":
			remote = false
		}
	}

	var requiredExperienceYears any
	if v, ok := fields["experience_level"]; ok && v != nil && !isEmptyFieldValue(v) {
		requiredExperienceYears = jobListingExperienceYears(fmt.Sprint(v))
	}

	var locationGeo any
	if geoSpaceID.Valid {
		locationGeo = geoSpaceID.String()
	}

	return map[string]any{
		"income":                    income,
		"cost":                      nil,
		"speed":                     nil,
		"contract_type":             contractType,
		"sponsorship":               sponsorship,
		"remote":                    remote,
		"required_experience_years": requiredExperienceYears,
		"language":                  []string{},
		"location_geo_space_id":     locationGeo,
	}
}

func isEmptyFieldValue(v any) bool {
	s, ok := v.(string)
	return ok && s == ""
}

func jobListingTruthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(x, "true") || x == "1"
	case int:
		return x != 0
	case int8:
		return x != 0
	case int16:
		return x != 0
	case int32:
		return x != 0
	case int64:
		return x != 0
	case uint:
		return x != 0
	case uint8:
		return x != 0
	case uint16:
		return x != 0
	case uint32:
		return x != 0
	case uint64:
		return x != 0
	case uintptr:
		return x != 0
	case float32:
		return x != 0
	case float64:
		return x != 0
	}
	return false
}

// jobListingExperienceYears maps the job board experience_level enum to a
// rough required-years estimate. A more precise mapping should be agreed with
// the product team; the current table is conservative and defaults to nil for
// unrecognized values.
func jobListingExperienceYears(level string) any {
	switch strings.ToLower(level) {
	case "entry":
		return 0
	case "mid":
		return 3
	case "senior":
		return 7
	case "lead":
		return 10
	case "executive":
		return 15
	}
	return nil
}
