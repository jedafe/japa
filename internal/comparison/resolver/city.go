package resolver

import (
	"context"
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
	var id pgtype.UUID
	var title string
	var desc, geoName, geoSlug *string

	switch r := row.(type) {
	case gen.GetCOLCityByIDRow:
		id, title, desc, geoName, geoSlug = r.ID, r.Title, r.Description, r.GeoSpaceName, r.GeoSpaceSlug
	case gen.ListCOLCitiesRow:
		id, title, desc, geoName, geoSlug = r.ID, r.Title, r.Description, r.GeoSpaceName, r.GeoSpaceSlug
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

	return ResolvedItem{
		Type:        "col_city",
		ID:          id.String(),
		Title:       title,
		Description: Excerpt(description, 160),
		Metadata: map[string]any{
			"geo_space_name": deref(geoName),
			"geo_space_slug": slug,
		},
	}
}
