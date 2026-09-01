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
func (r *JobListingResolver) Search(ctx context.Context, query string, filters map[string]any, tenantID pgtype.UUID, limit int32, offset int32) ([]ResolvedItem, error) {
	if r == nil || r.svc == nil {
		return nil, errors.New("job_listing resolver not initialized")
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

	emptyCursor := pgtype.Timestamptz{}
	fetchLimit := limit + offset
	if fetchLimit > content.MaxPageSize {
		fetchLimit = content.MaxPageSize
	}
	items, err := r.svc.ListContentsByType(ctx, tenantID, "job_listing", emptyCursor, fetchLimit, visibility.VisibilityFilter{})
	if err != nil {
		return nil, fmt.Errorf("listing job listings: %w", err)
	}

	var matched []ResolvedItem
	for _, it := range items {
		if query != "" && !containsQuery(it.Title, it.Slug, it.Body, query) {
			continue
		}
		matched = append(matched, jobListingFromAny(it, nil))
	}

	if int(offset) >= len(matched) {
		return []ResolvedItem{}, nil
	}
	end := int(offset) + int(limit)
	if end > len(matched) {
		end = len(matched)
	}
	return matched[offset:end], nil
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

	country := deref(targetCountry)
	if country == "" {
		country = deref(sourceCountry)
	}

	meta := map[string]any{
		"content_type": contentType,
		"slug":         slug,
		"status":       status,
		"visibility":   visibility,
		"score_inputs": jobListingScoreInputs(fieldValues, geoSpaceID, country),
	}
	if geoSpaceID.Valid {
		meta["geo_space_id"] = geoSpaceID.String()
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

func jobListingScoreInputs(fieldValues []byte, geoSpaceID pgtype.UUID, country string) map[string]any {
	fields := map[string]any{}
	if len(fieldValues) > 0 {
		_ = json.Unmarshal(fieldValues, &fields)
	}

	var income any
	if v, ok := fields["salary_max"]; ok && v != nil && !isEmptyFieldValue(v) {
		income = coerceFieldNumber(v)
	} else if v, ok := fields["salary_min"]; ok && v != nil && !isEmptyFieldValue(v) {
		income = coerceFieldNumber(v)
	}

	var cost any
	if v, ok := fields["relocation_package"]; ok && v != nil && !isEmptyFieldValue(v) {
		cost = coerceFieldNumber(v)
	} else if v, ok := fields["signing_bonus"]; ok && v != nil && !isEmptyFieldValue(v) {
		cost = coerceFieldNumber(v)
	}

	var speed any
	if v, ok := fields["time_to_start"]; ok && v != nil && !isEmptyFieldValue(v) {
		speed = coerceFieldNumber(v)
	} else if v, ok := fields["time_to_hire"]; ok && v != nil && !isEmptyFieldValue(v) {
		speed = coerceFieldNumber(v)
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
		if b, ok := v.(bool); ok {
			remote = b
		} else {
			s := fmt.Sprint(v)
			switch strings.ToLower(s) {
			case "remote", "hybrid":
				remote = true
			case "onsite":
				remote = false
			}
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

	language := languagesForGeoCode(country)

	return map[string]any{
		"income":                    income,
		"cost":                      cost,
		"speed":                     speed,
		"contract_type":             contractType,
		"sponsorship":               sponsorship,
		"remote":                    remote,
		"required_experience_years": requiredExperienceYears,
		"language":                  language,
		"location_geo_space_id":     locationGeo,
		"geo_space_id":              locationGeo,
	}
}

// coerceFieldNumber attempts to coerce a field value to a numeric any. It
// returns nil when the value is not a sensible number.
func coerceFieldNumber(v any) any {
	if n, ok := coerceNumber(v); ok {
		return n
	}
	return nil
}

func isEmptyFieldValue(v any) bool {
	s, ok := v.(string)
	return ok && s == ""
}

func jobListingTruthy(v any) bool {
	if b, ok := coerceBool(v); ok {
		return b
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
