package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/housingguide"
)

// housingGuideResolver resolves housing guides — registered so wiki field
// ops and JCE comparisons can target guide rows (§80 Tool Surface
// Hydration; jce_entity:housing_guide class assignments).
type housingGuideResolver struct {
	svc     *housingguide.Service
	queries *gen.Queries // optional: enables custom_field_values merge
}

// NewHousingGuideResolver creates a resolver for housing guides. Pass
// WithQueries to merge custom_field_values over rent_ranges (§80).
func NewHousingGuideResolver(svc *housingguide.Service, opts ...Option) Resolver {
	o := newResolverOptions(opts...)

	return &housingGuideResolver{svc: svc, queries: o.queries}
}

func (r *housingGuideResolver) Resolve(ctx context.Context, ref map[string]any) (ResolvedItem, bool, error) {
	idStr, ok := ref["id"].(string)
	if !ok || idStr == "" {
		return ResolvedItem{}, false, errors.New("housing_guide item_ref missing id")
	}

	id, err := ParseUUID(idStr)
	if err != nil {
		return ResolvedItem{}, false, err
	}

	guide, err := r.svc.GetByID(ctx, id)
	if err != nil {
		if IsNotFound(err) {
			return ResolvedItem{}, false, nil
		}

		return ResolvedItem{}, false, fmt.Errorf("fetching housing guide by id: %w", err)
	}

	// GetByID already merges cfv into guide.RentRanges (§80) — pass nil so
	// the item builder uses that merged bag without a second cfv read.
	return housingGuideToResolvedItem(guide, nil), true, nil
}

func (r *housingGuideResolver) Search(ctx context.Context, query string, filters map[string]any, limit, offset int32) ([]ResolvedItem, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	if offset < 0 {
		offset = 0
	}

	page := int32(1)
	if limit > 0 {
		page = offset/limit + 1
	}

	active := true

	// Scope the search with the geo/accommodation filters the comparison
	// scope chain provides — "country_id" maps to the destination space.
	destSpaceID := parseUUIDFilter(filters, "country_id")
	citySpaceID := parseUUIDFilter(filters, "city_id")

	var accommodationType string
	if v, ok := filters["accommodation_type"].(string); ok {
		accommodationType = v
	}

	guides, _, err := r.svc.AdminList(ctx, "published", accommodationType, query, destSpaceID, citySpaceID, &active, page, limit)
	if err != nil {
		return nil, fmt.Errorf("searching housing guides: %w", err)
	}

	// AdminList intentionally returns the stored bag (§80 admin surfaces),
	// so the resolver does its own batched cfv read for the page.
	var cfvByGuide map[pgtype.UUID]map[string]any

	if r.queries != nil && len(guides) > 0 {
		ids := make([]pgtype.UUID, 0, len(guides))
		for _, g := range guides {
			if id, perr := ParseUUID(g.ID); perr == nil {
				ids = append(ids, id)
			}
		}

		cfvByGuide = customfields.EntityFieldValuesForEntities(ctx, r.queries, "housing_guide", ids)
	}

	out := make([]ResolvedItem, 0, len(guides))
	for _, g := range guides {
		var cfv map[string]any

		if id, perr := ParseUUID(g.ID); perr == nil {
			cfv = cfvByGuide[id]
		}

		out = append(out, housingGuideToResolvedItem(g, cfv))
	}

	return out, nil
}

func housingGuideToResolvedItem(g housingguide.GuideDTO, cfv map[string]any) ResolvedItem {
	title := "Housing guide"
	if g.AccommodationType != "" {
		title = g.AccommodationType
	}
	if g.DestSpaceName != "" {
		title += " — " + g.DestSpaceName
	}
	if g.CitySpaceName != "" {
		title += " / " + g.CitySpaceName
	}

	// Merged field bag (rent_ranges || cfv) — cfv wins per key (§80).
	fields := rentRangesMap(g.RentRanges)

	if len(cfv) > 0 {
		for k, v := range cfv {
			fields[k] = v
		}
	}

	meta := map[string]any{
		"accommodation_type": g.AccommodationType,
		"score_inputs":       housingGuideScoreInputs(g, fields),
	}
	if len(fields) > 0 {
		meta["field_values"] = fields
	}
	if geo := guideGeoSpaceID(g); geo != "" {
		meta["geo_space_id"] = geo
	}
	if g.DestSpaceCode != "" {
		meta["country"] = g.DestSpaceCode
	}

	return ResolvedItem{
		Type:        "housing_guide",
		ID:          g.ID,
		Title:       title,
		Description: Excerpt(g.Tips, 160),
		Metadata:    meta,
	}
}

// guideGeoSpaceID prefers the city space when present — community edits
// target the most specific geo the guide serves.
func guideGeoSpaceID(g housingguide.GuideDTO) string {
	if g.CitySpaceID != "" {
		return g.CitySpaceID
	}

	return g.DestinationSpaceID
}

// rentRangesMap parses the rent_ranges JSONB bag into a mutable map.
func rentRangesMap(raw json.RawMessage) map[string]any {
	fields := map[string]any{}

	if len(raw) > 0 {
		var parsed map[string]any
		if err := json.Unmarshal(raw, &parsed); err == nil && parsed != nil {
			fields = parsed
		}
	}

	return fields
}

// housingGuideScoreInputs projects rent-related score inputs from the merged
// field bag so community-maintained values feed scoring (§80).
func housingGuideScoreInputs(g housingguide.GuideDTO, fields map[string]any) map[string]any {
	num := func(keys ...string) any {
		for _, k := range keys {
			if v, ok := fields[k]; ok && v != nil {
				if n, ok := coerceNumber(v); ok {
					return n
				}
			}
		}

		return nil
	}

	var geoID any
	if id := guideGeoSpaceID(g); id != "" {
		geoID = id
	}

	return map[string]any{
		"cost":           num("cost", "rent_monthly", "avg_rent_usd", "rent_usd"),
		"deposit_months": num("deposit_months", "deposit"),
		"geo_space_id":   geoID,
		"language":       []string{},
		"scam_awareness": len(g.ScamCategories) > 0,
	}
}
