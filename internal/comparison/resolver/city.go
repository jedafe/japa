package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/costofliving"
	"github.com/jedafe/japa/internal/db/gen"
)

// cityResolver resolves cost-of-living cities (comparison_items of the
// seeded cost_of_living type).
type cityResolver struct {
	store *costofliving.Store
}

// NewCityResolver creates a resolver for cost-of-living cities.
func NewCityResolver(store *costofliving.Store) Resolver {
	return &cityResolver{store: store}
}

func (r *cityResolver) Resolve(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, bool, error) {
	idStr, ok := ref["id"].(string)
	if !ok || idStr == "" {
		return ResolvedItem{}, false, errors.New("col_city item_ref missing id")
	}
	id, err := ParseUUID(idStr)
	if err != nil {
		return ResolvedItem{}, false, err
	}

	row, err := r.store.GetCityByID(ctx, tenantID, id)
	if err != nil {
		if IsNotFound(err) {
			return ResolvedItem{}, false, nil
		}
		return ResolvedItem{}, false, fmt.Errorf("fetching city by id: %w", err)
	}

	return cityRowToResolvedItem(row), true, nil
}

func (r *cityResolver) Search(ctx context.Context, query string, filters map[string]any, tenantID pgtype.UUID, limit int32, offset int32) ([]ResolvedItem, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}
	if offset < 0 {
		offset = 0
	}

	typeDef, err := r.store.GetCostOfLivingType(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("fetching cost_of_living type: %w", err)
	}

	var countryCode string
	if v, ok := filters["country_code"].(string); ok {
		countryCode = v
	}

	rows, err := r.store.ListCities(ctx, tenantID, typeDef.ID, query, countryCode, offset, limit)
	if err != nil {
		return nil, fmt.Errorf("searching cities: %w", err)
	}

	out := make([]ResolvedItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, cityRowToResolvedItem(row))
	}
	return out, nil
}

func cityRowToResolvedItem(row any) ResolvedItem {
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

	meta := map[string]any{
		"geo_space_name": deref(geoName),
		"geo_space_slug": slug,
		"score_inputs":   cityScoreInputs(geoSpaceID, intelCostData),
	}
	if geoSpaceID.Valid {
		meta["geo_space_id"] = geoSpaceID.String()
	}
	return ResolvedItem{
		Type:        "col_city",
		ID:          id.String(),
		Title:       title,
		Description: Excerpt(description, 160),
		Metadata:    meta,
	}
}

func cityScoreInputs(geoSpaceID pgtype.UUID, intelCostData []byte) map[string]any {
	costs := map[string]any{}
	if len(intelCostData) > 0 {
		var parsed map[string]any
		if err := json.Unmarshal(intelCostData, &parsed); err == nil {
			costs = parsed
		}
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
