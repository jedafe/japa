package resolver

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/spaces"
)

// universityResolver resolves spaces with the "university" space type.
type universityResolver struct {
	svc *spaces.Service
}

// NewUniversityResolver creates a resolver for university spaces.
func NewUniversityResolver(svc *spaces.Service) Resolver {
	return &universityResolver{svc: svc}
}

func (r *universityResolver) Resolve(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, bool, error) {
	idStr, _ := ref["id"].(string)
	slug, _ := ref["slug"].(string)

	if idStr == "" && slug == "" {
		return ResolvedItem{}, false, errors.New("university item_ref missing id or slug")
	}

	typeID, err := r.universityTypeID(ctx, tenantID)
	if err != nil {
		return ResolvedItem{}, false, err
	}
	if !typeID.Valid {
		return ResolvedItem{}, false, errors.New("no university space type found")
	}

	if idStr != "" {
		id, err := ParseUUID(idStr)
		if err != nil {
			return ResolvedItem{}, false, err
		}
		row, err := r.svc.GetSpaceByID(ctx, tenantID, id)
		if err != nil {
			if IsNotFound(err) {
				return ResolvedItem{}, false, nil
			}
			return ResolvedItem{}, false, fmt.Errorf("fetching university by id: %w", err)
		}
		if row.SpaceTypeID != typeID {
			return ResolvedItem{}, false, nil
		}
		return spaceRowToResolvedItem(row), true, nil
	}

	row, err := r.svc.GetSpaceBySlug(ctx, slug, tenantID)
	if err != nil {
		if IsNotFound(err) {
			return ResolvedItem{}, false, nil
		}
		return ResolvedItem{}, false, fmt.Errorf("fetching university by slug: %w", err)
	}
	if row.SpaceTypeID != typeID {
		return ResolvedItem{}, false, nil
	}
	return spaceRowToResolvedItem(row), true, nil
}

func (r *universityResolver) Search(ctx context.Context, query string, _ map[string]any, tenantID pgtype.UUID, limit int32) ([]ResolvedItem, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	typeID, err := r.universityTypeID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if !typeID.Valid {
		return nil, nil
	}

	items, err := r.svc.ListSpacesByTypePaginated(ctx, tenantID, typeID, query, "", "name", limit, 0)
	if err != nil {
		return nil, fmt.Errorf("searching universities: %w", err)
	}

	out := make([]ResolvedItem, 0, len(items))
	for _, it := range items {
		out = append(out, spaceRowToResolvedItem(it))
	}
	return out, nil
}

func (r *universityResolver) universityTypeID(ctx context.Context, tenantID pgtype.UUID) (pgtype.UUID, error) {
	t, err := r.svc.GetSpaceTypeBySlug(ctx, "university", tenantID)
	if err != nil {
		if IsNotFound(err) {
			return pgtype.UUID{}, nil
		}
		return pgtype.UUID{}, fmt.Errorf("getting university space type: %w", err)
	}
	return t.ID, nil
}

func spaceRowToResolvedItem(row any) ResolvedItem {
	var id pgtype.UUID
	var name, slug string
	var description *string
	var geoCode *string
	var level string
	var logoMediaID, headerImageID pgtype.UUID
	var memberCount int32

	switch r := row.(type) {
	case gen.GetSpaceByIDRow:
		id, name, slug, description, geoCode, level, logoMediaID, headerImageID, memberCount = r.ID, r.Name, r.Slug, r.Description, r.GeoCode, r.Level, r.LogoMediaID, r.HeaderImageID, r.MemberCount
	case gen.GetSpaceBySlugRow:
		id, name, slug, description, geoCode, level, logoMediaID, headerImageID, memberCount = r.ID, r.Name, r.Slug, r.Description, r.GeoCode, r.Level, r.LogoMediaID, r.HeaderImageID, r.MemberCount
	case gen.ListSpacesByTypePaginatedRow:
		id, name, slug, description, geoCode, level, logoMediaID, headerImageID, memberCount = r.ID, r.Name, r.Slug, r.Description, r.GeoCode, r.Level, r.LogoMediaID, r.HeaderImageID, r.MemberCount
	}

	return makeSpaceResolvedItem("university", id, name, slug, description, geoCode, level, logoMediaID, headerImageID, memberCount)
}

func makeSpaceResolvedItem(entityType string, id pgtype.UUID, name, slug string, description *string, geoCode *string, level string, logoMediaID, headerImageID pgtype.UUID, memberCount int32) ResolvedItem {
	desc := ""
	if description != nil {
		desc = *description
	}
	imageID := ""
	if logoMediaID.Valid {
		imageID = logoMediaID.String()
	} else if headerImageID.Valid {
		imageID = headerImageID.String()
	}
	meta := map[string]any{
		"geo_space_id": id.String(),
		"slug":         slug,
		"space":        true,
		"level":        level,
		"geo_code":     deref(geoCode),
		"image_id":     imageID,
		"member_count": memberCount,
		"score_inputs": spaceScoreInputsForType(entityType, id),
	}
	if level == "country" && geoCode != nil && *geoCode != "" {
		meta["country"] = *geoCode
	}
	return ResolvedItem{
		Type:        entityType,
		ID:          id.String(),
		Title:       name,
		Description: Excerpt(desc, 160),
		Metadata:    meta,
	}
}

func spaceScoreInputsForType(entityType string, id pgtype.UUID) map[string]any {
	if entityType == "university" {
		return map[string]any{
			"cost":                   nil,
			"income":                 nil,
			"acceptance_rate":        nil,
			"duration_years":         nil,
			"qs_ranking":             nil,
			"pr_path_ease":           nil,
			"post_study_work_months": nil,
			"language":               []string{},
			"accreditation":          []string{},
			"fields_of_study":        []string{},
			"geo_space_id":           id.String(),
		}
	}
	return map[string]any{
		"cost":             nil,
		"income":           nil,
		"speed":            nil,
		"pr_path_ease":     nil,
		"safety_index":     nil,
		"healthcare_index": nil,
		"language":         []string{},
		"geo_space_id":     id.String(),
	}
}
