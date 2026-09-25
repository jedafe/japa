// Package costofliving implements the Cost of Living Calculator (Section 72,
// ADR-072). It is a specialised comparison type built on the §30 comparison
// system: city data lives in comparison_items under a seeded cost_of_living
// comparison type, with per-attribute values in comparison_values. Lagos is
// the default baseline (is_baseline = true).
//
// The store is the data-access layer — it wraps the sqlc-generated Queries
// and contains no business logic.
//
// Ref: japaconnect-engineering/sections/72-cost-of-living.md
//
//	japaconnect-engineering/adr/ADR-072-cost-of-living-calculator.md
package costofliving

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated Queries for cost_of_living operations.
// It is the data-access layer — no business logic here.
type Store struct {
	queries *gen.Queries
	pool    *db.DB
}

// NewStore creates a cost-of-living store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Reader),
		pool:    database,
	}
}

// GetCostOfLivingType returns the seeded cost_of_living comparison type
// definition (with config JSONB containing the weighted cost index weights).
func (s *Store) GetCostOfLivingType(ctx context.Context) (gen.ComparisonTypeDefinition, error) {
	t, err := s.queries.GetCostOfLivingType(ctx)
	if err != nil {
		return gen.ComparisonTypeDefinition{}, fmt.Errorf("costofliving: get type: %w", err)
	}

	return t, nil
}

// ListCities lists cities (comparison_items) with cost_of_living data,
// paginated and searchable by title, optionally filtered by country geo_code,
// country_id, state_id, and city_id (via the space_parent_spaces hierarchy).
func (s *Store) ListCities(ctx context.Context, typeID pgtype.UUID, search, countryCode string, countryID, stateID, cityID pgtype.UUID, offset, limit int32) ([]gen.ListCOLCitiesRow, error) {
	rows, err := s.queries.ListCOLCities(ctx, gen.ListCOLCitiesParams{

		TypeID:      typeID,
		Search:      search,
		CountryCode: countryCode,
		CountryID:   countryID,
		StateID:     stateID,
		CityID:      cityID,
		PageOffset:  offset,
		PageLimit:   limit,
	})
	if err != nil {
		return nil, fmt.Errorf("costofliving: list cities: %w", err)
	}

	return rows, nil
}

// CountCities returns the total number of cities matching the search and
// optional country geo_code / ancestor scope filters.
func (s *Store) CountCities(ctx context.Context, typeID pgtype.UUID, search, countryCode string, countryID, stateID, cityID pgtype.UUID) (int64, error) {
	count, err := s.queries.CountCOLCities(ctx, gen.CountCOLCitiesParams{

		TypeID:      typeID,
		Search:      search,
		CountryCode: countryCode,
		CountryID:   countryID,
		StateID:     stateID,
		CityID:      cityID,
	})
	if err != nil {
		return 0, fmt.Errorf("costofliving: count cities: %w", err)
	}

	return count, nil
}

// GetCityByGeoSpaceID returns the cost_of_living comparison item for the
// given geo space, if one exists. It is used by the JCE resolver to
// backfill score_inputs for spaces.
func (s *Store) GetCityByGeoSpaceID(ctx context.Context, geoSpaceID pgtype.UUID) (gen.GetCOLByGeoSpaceIDRow, error) {
	row, err := s.queries.GetCOLByGeoSpaceID(ctx,

		geoSpaceID,
	)
	if err != nil {
		return gen.GetCOLByGeoSpaceIDRow{}, fmt.Errorf("costofliving: get city by geo space id: %w", err)
	}

	return row, nil
}

// GetBaselineCity returns the baseline city (is_baseline = true) for the
// cost_of_living type, with its geo space info.
func (s *Store) GetBaselineCity(ctx context.Context, typeID pgtype.UUID) (gen.GetCOLBaselineCityRow, error) {
	row, err := s.queries.GetCOLBaselineCity(ctx,

		typeID,
	)
	if err != nil {
		return gen.GetCOLBaselineCityRow{}, fmt.Errorf("costofliving: get baseline city: %w", err)
	}

	return row, nil
}

// GetCityByID returns a single city (comparison_item) by ID with its geo
// space info.
func (s *Store) GetCityByID(ctx context.Context, id pgtype.UUID) (gen.GetCOLCityByIDRow, error) {
	row, err := s.queries.GetCOLCityByID(ctx,

		id,
	)
	if err != nil {
		return gen.GetCOLCityByIDRow{}, fmt.Errorf("costofliving: get city by id: %w", err)
	}

	return row, nil
}

// GetCitiesByIDs batch-fetches multiple cities by ID. Used by CompareCities
// to avoid an N+1 (one GetCityByID call per requested city).
func (s *Store) GetCitiesByIDs(ctx context.Context, ids []pgtype.UUID) ([]gen.GetCOLCitiesByIDsRow, error) {
	rows, err := s.queries.GetCOLCitiesByIDs(ctx,

		ids,
	)
	if err != nil {
		return nil, fmt.Errorf("costofliving: get cities by ids: %w", err)
	}

	return rows, nil
}

// GetValuesForCities returns the comparison values (cost data matrix) for a
// set of city item IDs. The service layer pivots these into a matrix.
func (s *Store) GetValuesForCities(ctx context.Context, itemIDs []pgtype.UUID) ([]gen.GetCOLValuesForCitiesRow, error) {
	rows, err := s.queries.GetCOLValuesForCities(ctx,

		itemIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("costofliving: get values for cities: %w", err)
	}

	return rows, nil
}

// ListSpaceFieldValues returns community field values keyed by geo space ID
// (entity_type='space') in one batched custom_field_values read — the
// canonical source for the §80 Tool Surface Hydration overlay in
// CompareCities.
func (s *Store) ListSpaceFieldValues(ctx context.Context, spaceIDs []pgtype.UUID) map[pgtype.UUID]map[string]any {
	return customfields.EntityFieldValuesForEntities(ctx, s.queries, "space", spaceIDs)
}

// AdminStats returns the per-city data completeness + staleness report.
func (s *Store) AdminStats(ctx context.Context, typeID pgtype.UUID, offset, limit int32) ([]gen.AdminCOLStatsRow, error) {
	rows, err := s.queries.AdminCOLStats(ctx, gen.AdminCOLStatsParams{

		TypeID:     typeID,
		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("costofliving: admin stats: %w", err)
	}

	return rows, nil
}
