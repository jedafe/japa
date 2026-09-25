package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/costofliving"
	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db/gen"
)

// cityResolver resolves cost-of-living cities (comparison_items of the
// seeded cost_of_living type).
type cityResolver struct {
	store   *costofliving.Store
	queries *gen.Queries // optional: enables custom_field_values merge
}

// NewCityResolver creates a resolver for cost-of-living cities. Pass
// WithQueries to merge the linked geo space's custom_field_values over
// intel_cost_data (§80 Tool Surface Hydration).
func NewCityResolver(store *costofliving.Store, opts ...Option) Resolver {
	o := newResolverOptions(opts...)

	return &cityResolver{store: store, queries: o.queries}
}

func (r *cityResolver) Resolve(ctx context.Context, ref map[string]any) (ResolvedItem, bool, error) {
	idStr, ok := ref["id"].(string)
	if !ok || idStr == "" {
		return ResolvedItem{}, false, errors.New("col_city item_ref missing id")
	}

	id, err := ParseUUID(idStr)
	if err != nil {
		return ResolvedItem{}, false, err
	}

	row, err := r.store.GetCityByID(ctx, id)
	if err != nil {
		if IsNotFound(err) {
			return ResolvedItem{}, false, nil
		}

		return ResolvedItem{}, false, fmt.Errorf("fetching city by id: %w", err)
	}

	return cityRowToResolvedItem(row, r.cityFieldValues(ctx, row.GeoSpaceID)), true, nil
}

func (r *cityResolver) Search(ctx context.Context, query string, filters map[string]any, limit, offset int32) ([]ResolvedItem, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	if offset < 0 {
		offset = 0
	}

	typeDef, err := r.store.GetCostOfLivingType(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching cost_of_living type: %w", err)
	}

	var countryCode string

	var countryID, stateID, cityID pgtype.UUID

	if v, ok := filters["country_code"].(string); ok {
		countryCode = v
	}

	countryID = parseUUIDFilter(filters, "country_id")
	stateID = parseUUIDFilter(filters, "state_id")
	cityID = parseUUIDFilter(filters, "city_id")

	rows, err := r.store.ListCities(ctx, typeDef.ID, query, countryCode, countryID, stateID, cityID, offset, limit)
	if err != nil {
		return nil, fmt.Errorf("searching cities: %w", err)
	}

	// One batched cfv read for the page — community values address the geo
	// space, so the batch keys on geo_space_id, not the item ID (§80).
	var cfvBySpace map[pgtype.UUID]map[string]any

	if r.queries != nil && len(rows) > 0 {
		spaceIDs := make([]pgtype.UUID, 0, len(rows))
		seen := make(map[pgtype.UUID]bool, len(rows))

		for _, row := range rows {
			if row.GeoSpaceID.Valid && !seen[row.GeoSpaceID] {
				seen[row.GeoSpaceID] = true

				spaceIDs = append(spaceIDs, row.GeoSpaceID)
			}
		}

		cfvBySpace = customfields.EntityFieldValuesForEntities(ctx, r.queries, "space", spaceIDs)
	}

	out := make([]ResolvedItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, cityRowToResolvedItem(row, cfvBySpace[row.GeoSpaceID]))
	}

	return out, nil
}

// cityFieldValues fetches the community field values recorded against the
// city's geo space (entity_type='space', entity_id=geo_space_id) — community
// edits target the space, not the comparison_items row (§80).
func (r *cityResolver) cityFieldValues(ctx context.Context, geoSpaceID pgtype.UUID) map[string]any {
	return customfields.EntityFieldValues(ctx, r.queries, "space", geoSpaceID)
}

func cityRowToResolvedItem(row any, cfv map[string]any) ResolvedItem {
	var id, geoSpaceID pgtype.UUID

	var title string

	var desc, geoName, geoSlug *string

	var intelCostData []byte

	switch r := row.(type) {
	case gen.GetCOLCityByIDRow:
		id, title, desc, geoSpaceID, geoName, geoSlug, intelCostData = r.ID, r.Title, r.Description, r.GeoSpaceID, r.GeoSpaceName, r.GeoSpaceSlug, r.IntelCostData
	case gen.ListCOLCitiesRow:
		id, title, desc, geoSpaceID, geoName, geoSlug, intelCostData = r.ID, r.Title, r.Description, r.GeoSpaceID, r.GeoSpaceName, r.GeoSpaceSlug, r.IntelCostData
	default:
		return ResolvedItem{Type: "col_city"}
	}

	description := ""
	if desc != nil {
		description = *desc
	}

	if geoName != nil {
		if description != "" {
			description += " · "
		}

		description += *geoName
	}

	slug := ""
	if geoSlug != nil {
		slug = *geoSlug
	}

	// §80 Tool Surface Hydration: community values recorded on the city's geo
	// space overlay intel_cost_data — cfv wins per key — so approved wiki
	// edits feed score inputs and matrix hydration on next read.
	costs := intelCostDataMap(intelCostData)

	if len(cfv) > 0 {
		for k, v := range cfv {
			costs[k] = v
		}
	}

	meta := map[string]any{
		"geo_space_name": deref(geoName),
		"geo_space_slug": slug,
		"score_inputs":   cityScoreInputs(geoSpaceID, costs),
	}
	if geoSpaceID.Valid {
		meta["geo_space_id"] = geoSpaceID.String()
	}

	if len(costs) > 0 {
		meta["field_values"] = costs
	}

	return ResolvedItem{
		Type:        "col_city",
		ID:          id.String(),
		Title:       title,
		Description: Excerpt(description, 160),
		Metadata:    meta,
	}
}

// intelCostDataMap parses a comparison_items.intel_cost_data JSONB blob into
// a mutable map so community field values can be overlaid onto it (§80).
func intelCostDataMap(intelCostData []byte) map[string]any {
	costs := map[string]any{}

	if len(intelCostData) > 0 {
		var parsed map[string]any
		if err := json.Unmarshal(intelCostData, &parsed); err == nil {
			costs = parsed
		}
	}

	return costs
}

func cityScoreInputs(geoSpaceID pgtype.UUID, costs map[string]any) map[string]any {
	if costs == nil {
		costs = map[string]any{}
	}

	getNumeric := func(key string) any {
		if v, ok := costs[key]; ok && v != nil {
			if n, ok := coerceNumber(v); ok {
				return n
			}
		}

		return nil
	}

	var geoID any
	if geoSpaceID.Valid {
		geoID = geoSpaceID.String()
	}

	// Explicit cost index, if the intelligence JSON carries one.
	cost := getNumeric("cost")

	// Otherwise fall back to summing the major monthly cost categories.
	if cost == nil {
		monthlyKeys := []string{"rent_1br_city_centre", "rent_1br_outside", "groceries_monthly", "transport_monthly", "utilities_monthly", "internet_monthly", "gym_monthly"}
		hasAny := false
		total := 0.0

		for i, k := range monthlyKeys {
			if i == 1 {
				// Only count one rent category; prefer city centre if present.
				if _, ok := costs["rent_1br_city_centre"]; ok {
					continue
				}
			}

			if v := getNumeric(k); v != nil {
				if f, ok := v.(float64); ok {
					total += f
					hasAny = true
				}

				if in, ok := v.(int); ok {
					total += float64(in)
					hasAny = true
				}

				if in, ok := v.(int64); ok {
					total += float64(in)
					hasAny = true
				}
			}
		}

		if hasAny {
			cost = total
		}
	}

	// Prefer 1BR city centre rent; fall back to outside centre.
	rent := getNumeric("rent_1br_city_centre")
	if rent == nil {
		rent = getNumeric("rent_1br_outside")
	}

	return map[string]any{
		"cost":             cost,
		"rent":             rent,
		"groceries":        getNumeric("groceries_monthly"),
		"transport":        getNumeric("transport_monthly"),
		"utilities":        getNumeric("utilities_monthly"),
		"income":           getNumeric("avg_net_salary"),
		"language":         []string{},
		"safety_index":     getNumeric("safety_index"),
		"healthcare_index": getNumeric("healthcare_index"),
		"geo_space_id":     geoID,
	}
}
