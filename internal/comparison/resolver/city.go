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

func (r *cityResolver) Search(ctx context.Context, query string, filters map[string]any, tenantID pgtype.UUID, limit int32) ([]ResolvedItem, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	typeDef, err := r.store.GetCostOfLivingType(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("fetching cost_of_living type: %w", err)
	}

	var countryCode string
	if v, ok := filters["country_code"].(string); ok {
		countryCode = v
	}

	rows, err := r.store.ListCities(ctx, tenantID, typeDef.ID, query, countryCode, 0, limit)
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

	get := func(key string) any {
		if v, ok := costs[key]; ok && v != nil {
			return v
		}
		return nil
	}

	var geoID any
	if geoSpaceID.Valid {
		geoID = geoSpaceID.String()
	}

	return map[string]any{
		"cost":             nil,
		"rent":             get("rent_1br_city_centre"),
		"groceries":        get("groceries_monthly"),
		"transport":        get("transport_monthly"),
		"utilities":        get("utilities_monthly"),
		"income":           get("avg_net_salary"),
		"language":         []string{},
		"safety_index":     nil,
		"healthcare_index": nil,
		"geo_space_id":     geoID,
	}
}
