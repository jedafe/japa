// Package costofliving — service.go holds the business logic for the Cost of
// Living Calculator. It reads col_calculator.* system_settings via the
// hot-reloadable config.Settings and computes the comparison matrix with
// ratios vs the baseline city and a weighted Overall Cost Index.
//
// Ref: japaconnect-engineering/sections/72-cost-of-living.md
//
//	japaconnect-engineering/adr/ADR-072-cost-of-living-calculator.md
package costofliving

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
)

// Default system_settings values (Section 72). These are used when a setting
// is unset or unparseable — see config.Settings.GetBoolOr/GetIntOr.
const (
	defaultEnabled   = true
	defaultStaleDays = 180
	defaultMaxCities = 3
)

// ErrCalculatorDisabled is returned by service methods when the
// col_calculator.enabled system_settings toggle is false. Handlers translate
// it into a 404 `col_calculator_disabled` response per ADR-042 (feature-flag
// middleware returns 404, not 403/500, when a feature is disabled).
var ErrCalculatorDisabled = errors.New("costofliving: calculator disabled")

// CityData is a single city row in the comparison matrix. Values maps
// dimension key (dimension_id string) → raw value as stored (text).
// IsBaseline marks the city that ratios in this matrix were computed
// against — the user-selected baseline when baseline_id was passed, else
// the seeded is_baseline item (§72 amendment: user-selectable baseline).
type CityData struct {
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	GeoSpaceName string            `json:"geo_space_name"`
	GeoSpaceSlug string            `json:"geo_space_slug"`
	IsBaseline   bool              `json:"is_baseline"`
	LastUpdated  string            `json:"last_updated"`
	Values       map[string]string `json:"values"`
	// CurrencyCode is the city's resolved country currency (§58). Converted
	// marks whether Values matrix cells were normalized to DisplayCurrency.
	CurrencyCode string `json:"currency_code,omitempty"`
	Converted    bool   `json:"converted"`
}

// CategoryData is a single cost category (dimension) across all cities.
// Values maps city ID → raw numeric value; Ratios maps city ID → ratio vs
// the baseline city's value for this category.
type CategoryData struct {
	Key    string             `json:"key"`
	Label  string             `json:"label"`
	Values map[string]float64 `json:"values"`
	Ratios map[string]float64 `json:"ratios"`
}

// ComparisonMatrix is the full comparison result returned by CompareCities.
// OverallIndex maps city ID → weighted Overall Cost Index (1.0 = baseline).
// DisplayCurrency is the fx base currency all converted values are
// denominated in (empty when FX is unavailable — values stay local).
type ComparisonMatrix struct {
	Cities          []CityData         `json:"cities"`
	Categories      []CategoryData     `json:"categories"`
	BaselineCityID  string             `json:"baseline_city_id"`
	OverallIndex    map[string]float64 `json:"overall_index"`
	DisplayCurrency string             `json:"display_currency"`
	FxFetchedAt     string             `json:"fx_fetched_at,omitempty"`
	FxStale         bool               `json:"fx_stale,omitempty"`
}

// CityListItem is a single city in the paginated list response.
type CityListItem struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	GeoSpaceName string `json:"geo_space_name"`
	GeoSpaceSlug string `json:"geo_space_slug"`
	CurrencyCode string `json:"currency_code"`
	IsBaseline   bool   `json:"is_baseline"`
	LastUpdated  string `json:"last_updated"`
	Position     int32  `json:"position"`
}

// CitiesListResponse is the paginated list response for ListCities.
type CitiesListResponse struct {
	Cities []CityListItem `json:"cities"`
	Total  int64          `json:"total"`
	Page   int            `json:"page"`
	Limit  int            `json:"limit"`
}

// AdminStatsItem is a single city row in the admin stats report.
type AdminStatsItem struct {
	CityID           string  `json:"city_id"`
	CityName         string  `json:"city_name"`
	IsBaseline       bool    `json:"is_baseline"`
	LastUpdated      string  `json:"last_updated"`
	IsStale          bool    `json:"is_stale"`
	TotalDimensions  int64   `json:"total_dimensions"`
	FilledDimensions int64   `json:"filled_dimensions"`
	CompletenessPct  float64 `json:"completeness_pct"`
}

// AdminStatsResponse is the paginated admin stats report.
type AdminStatsResponse struct {
	Cities []AdminStatsItem `json:"cities"`
	Total  int64            `json:"total"`
	Page   int              `json:"page"`
	Limit  int              `json:"limit"`
}

// FeatureChecker resolves feature flag state from the feature_flags table —
// the single source of truth for feature reachability.
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
}

// Service holds the business logic for the cost-of-living calculator.
type Service struct {
	store    *Store
	settings *config.Settings
	checker  FeatureChecker
	fxRates  FxRateProvider
}

// FxRateProvider is the narrow slice of *fx.Service the calculator needs —
// injected via SetFxRateProvider; nil keeps values in local currency.
type FxRateProvider interface {
	BaseRate(ctx context.Context, quote string) (rate float64, base string, fetchedAt time.Time, stale bool, err error)
}

// SetFxRateProvider injects the FX rate source for base-currency
// normalization of the comparison matrix.
func (s *Service) SetFxRateProvider(p FxRateProvider) { s.fxRates = p }

// dimensionTypeCurrency mirrors comparison.DimensionTypeCurrency —
// declared locally because comparison/resolver imports this package.
const dimensionTypeCurrency = "currency"

// NewService creates a cost-of-living service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// SetFeatureChecker injects the feature flag checker so the
// col_calculator.enabled reachability check reads the feature_flags table
// (the single source of truth) instead of system_settings. Falls back to
// system_settings when nil.
func (s *Service) SetFeatureChecker(c FeatureChecker) { s.checker = c }

// calculatorEnabled resolves the col_calculator.enabled feature flag.
// system_settings is consulted only as a fallback when no checker is wired.
func (s *Service) calculatorEnabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "col_calculator.enabled")
	}

	if s.settings == nil {
		return defaultEnabled
	}

	return s.settings.GetBoolOr(ctx, "col_calculator.enabled", defaultEnabled)
}

// IsStale reports whether the given last_updated_at timestamp is older than
// the col_calculator.stale_days threshold. Section 72: default 180 days.
func (s *Service) IsStale(ctx context.Context, lastUpdatedAt time.Time) bool {
	staleDays := defaultStaleDays
	if s.settings != nil {
		staleDays = s.settings.GetIntOr(ctx, "col_calculator.stale_days", defaultStaleDays)
	}

	cutoff := time.Now().AddDate(0, 0, -staleDays)

	return lastUpdatedAt.Before(cutoff)
}

// IsStaleAt reports whether the given timestamp is older than staleDays.
// This is the pure variant used by tests that do not need config resolution.
func IsStaleAt(lastUpdatedAt, now time.Time, staleDays int) bool {
	cutoff := now.AddDate(0, 0, -staleDays)
	return lastUpdatedAt.Before(cutoff)
}

// maxCities returns the configured col_calculator.max_cities limit.
func (s *Service) maxCities(ctx context.Context) int {
	if s.settings == nil {
		return defaultMaxCities
	}

	return s.settings.GetIntOr(ctx, "col_calculator.max_cities", defaultMaxCities)
}

// ListCities returns a paginated list of cities with cost_of_living data.
// Public route (col_calculator.use — all users including anonymous).
// The optional countryCode is an ISO 3166-1 alpha-2 country code used to
// filter cities to those in the selected primary geo space.
func (s *Service) ListCities(ctx context.Context, search, countryCode string, page, limit int) (CitiesListResponse, error) {
	if !s.calculatorEnabled(ctx) {
		return CitiesListResponse{}, ErrCalculatorDisabled
	}

	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := int32((page - 1) * limit) // #nosec

	typeDef, err := s.store.GetCostOfLivingType(ctx)
	if err != nil {
		return CitiesListResponse{}, fmt.Errorf("costofliving: list cities: %w", err)
	}

	rows, err := s.store.ListCities(ctx, typeDef.ID, search, countryCode, pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, offset, int32(limit)) // #nosec
	if err != nil {
		return CitiesListResponse{}, err
	}

	total, err := s.store.CountCities(ctx, typeDef.ID, search, countryCode, pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		return CitiesListResponse{}, err
	}

	cities := make([]CityListItem, 0, len(rows))

	for _, r := range rows {
		desc := ""
		if r.Description != nil {
			desc = *r.Description
		}

		geoName := ""
		if r.GeoSpaceName != nil {
			geoName = *r.GeoSpaceName
		}

		geoSlug := ""
		if r.GeoSpaceSlug != nil {
			geoSlug = *r.GeoSpaceSlug
		}

		lastUpdated := ""
		if r.LastUpdatedAt.Valid {
			lastUpdated = r.LastUpdatedAt.Time.UTC().Format(time.RFC3339)
		}

		currency := ""
		if r.CurrencyCode != nil {
			currency = *r.CurrencyCode
		}

		cities = append(cities, CityListItem{
			ID:           r.ID.String(),
			Title:        r.Title,
			Description:  desc,
			GeoSpaceName: geoName,
			GeoSpaceSlug: geoSlug,
			CurrencyCode: currency,
			IsBaseline:   r.IsBaseline,
			LastUpdated:  lastUpdated,
			Position:     r.Position,
		})
	}

	return CitiesListResponse{
		Cities: cities,
		Total:  total,
		Page:   page,
		Limit:  limit,
	}, nil
}

// ResolveCityRef converts one city_ids/baseline_id token to a comparison
// item ID. UUIDs pass through; anything else is resolved as the city's
// geo-space slug (the same public identifier used by
// /wiki/{slug}/cost-of-living links) so share URLs stay human-readable.
// Returns Valid=false when the token resolves to nothing.
func (s *Service) ResolveCityRef(ctx context.Context, token string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(token); err == nil {
		return id, nil
	}

	if s.store == nil {
		return pgtype.UUID{}, nil
	}

	id, err := s.store.GetCityIDBySpaceSlug(ctx, token)
	if err != nil {
		return pgtype.UUID{}, nil
	}

	return id, nil
}

// baselineRef normalizes the city row that serves as the comparison
// reference — either a user-selected city (baseline_id param) or the seeded
// is_baseline item — so the matrix builder treats both sources uniformly.
type baselineRef struct {
	ID            pgtype.UUID
	Title         string
	GeoSpaceID    pgtype.UUID
	GeoSpaceName  *string
	GeoSpaceSlug  *string
	CurrencyCode  *string
	LastUpdatedAt pgtype.Timestamptz
}

// resolveBaseline returns the effective comparison baseline. When the
// caller passes baseline_id naming a live cost_of_living item it wins
// (§72 amendment: user-selectable baseline); anything else — absent,
// unresolved, or an item from another comparison type — falls back to the
// seeded is_baseline city.
func (s *Service) resolveBaseline(ctx context.Context, typeID, requested pgtype.UUID) (baselineRef, error) {
	if requested.Valid {
		row, err := s.store.GetCityByID(ctx, requested)
		if err == nil && row.TypeID == typeID {
			return baselineRef{
				ID:            row.ID,
				Title:         row.Title,
				GeoSpaceID:    row.GeoSpaceID,
				GeoSpaceName:  row.GeoSpaceName,
				GeoSpaceSlug:  row.GeoSpaceSlug,
				CurrencyCode:  row.CurrencyCode,
				LastUpdatedAt: row.LastUpdatedAt,
			}, nil
		}

		if err != nil {
			slog.Warn("costofliving: baseline_id lookup failed, using seeded baseline", "baseline_id", requested.String(), "error", err)
		} else {
			slog.Warn("costofliving: baseline_id is not a cost_of_living item, using seeded baseline", "baseline_id", requested.String())
		}
	}

	row, err := s.store.GetBaselineCity(ctx, typeID)
	if err != nil {
		return baselineRef{}, fmt.Errorf("costofliving: compare: get baseline: %w", err)
	}

	return baselineRef{
		ID:            row.ID,
		Title:         row.Title,
		GeoSpaceID:    row.GeoSpaceID,
		GeoSpaceName:  row.GeoSpaceName,
		GeoSpaceSlug:  row.GeoSpaceSlug,
		CurrencyCode:  row.CurrencyCode,
		LastUpdatedAt: row.LastUpdatedAt,
	}, nil
}

// CompareCities returns a comparison matrix with raw values + ratios vs the
// baseline city and a weighted Overall Cost Index. Steps per ADR-072 §2:
//  1. Get the cost_of_living type definition (for config weights).
//  2. Resolve the baseline city (baseline_id param or seeded is_baseline).
//  3. Get values for all requested cities + the baseline.
//  4. Pivot values into a matrix.
//  5. Compute ratios vs baseline for each category.
//  6. Compute weighted Overall Cost Index using config weights.
//
// The baseline city is always included in the result even if not in cityIDs.
func (s *Service) CompareCities(ctx context.Context, cityIDs []pgtype.UUID, baselineID pgtype.UUID) (ComparisonMatrix, error) {
	if !s.calculatorEnabled(ctx) {
		return ComparisonMatrix{}, ErrCalculatorDisabled
	}

	maxCities := s.maxCities(ctx)

	// Deduplicate city IDs before the max_cities check so that duplicate
	// IDs in the query string don't trigger a false rejection or produce
	// duplicate entries in the result.
	seen := make(map[pgtype.UUID]bool, len(cityIDs))
	uniqueIDs := make([]pgtype.UUID, 0, len(cityIDs))

	for _, id := range cityIDs {
		if id.Valid && !seen[id] {
			seen[id] = true

			uniqueIDs = append(uniqueIDs, id)
		}
	}

	// Step 1: get the cost_of_living type definition (config holds weights).
	// s.store is nil in unit tests that exercise the max_cities guard in
	// isolation — skip DB-backed resolution there and exempt only the
	// requested baseline_id.
	var typeDef gen.ComparisonTypeDefinition

	var baseline baselineRef

	if s.store != nil {
		var err error

		typeDef, err = s.store.GetCostOfLivingType(ctx)
		if err != nil {
			return ComparisonMatrix{}, fmt.Errorf("costofliving: compare: get type: %w", err)
		}

		// Step 2: resolve the baseline city — a caller-supplied baseline_id
		// that names a live cost_of_living item wins; otherwise the seeded
		// baseline.
		baseline, err = s.resolveBaseline(ctx, typeDef.ID, baselineID)
		if err != nil {
			return ComparisonMatrix{}, err
		}

		baselineID = baseline.ID
	}

	// max_cities counts destination cities only (ADR-072: "1–N destination
	// cities plus the baseline"). The effective baseline never consumes a
	// slot — including a seeded baseline smuggled inside city_ids by a
	// legacy share URL that predates the baseline_id param.
	destCount := 0

	for _, id := range uniqueIDs {
		if baselineID.Valid && id == baselineID {
			continue
		}

		destCount++
	}

	if destCount > maxCities {
		return ComparisonMatrix{}, fmt.Errorf("too many cities: %d destinations (max %d)", destCount, maxCities)
	}

	cityIDs = uniqueIDs

	weights := parseWeights(typeDef.Config)

	// Step 3: collect the set of city IDs to fetch values for (requested +
	// baseline, deduplicated). The baseline is always included.
	wantSet := make(map[pgtype.UUID]bool, len(cityIDs)+1)
	wantSet[baselineID] = true

	for _, id := range cityIDs {
		if id.Valid {
			wantSet[id] = true
		}
	}

	allIDs := make([]pgtype.UUID, 0, len(wantSet))
	for id := range wantSet {
		allIDs = append(allIDs, id)
	}

	values, err := s.store.GetValuesForCities(ctx, allIDs)
	if err != nil {
		return ComparisonMatrix{}, fmt.Errorf("costofliving: compare: get values: %w", err)
	}

	// Step 4: pivot values into a matrix.
	// dimValues[dimensionID][cityID] = float64 value (parsed from text).
	// dimMeta[dimensionID] = {label, key, slug, sourceKey}.
	// sourceKey is the bound custom_fields.key (source_field_id) that
	// community wiki values are keyed under (§80 Tool Surface Hydration).
	type dimMeta struct {
		Label     string
		Key       string
		Slug      string
		SourceKey string
		// Money marks currency dimensions — only these normalize to the
		// fx base; ratings/numbers/text pass through unchanged.
		Money bool
	}

	dimValues := make(map[string]map[string]float64)
	dimOrder := make([]string, 0)
	dimMetaMap := make(map[string]dimMeta)
	cityRawValues := make(map[string]map[string]string)

	for _, v := range values {
		dimKey := v.DimensionID.String()
		cityKey := v.ItemID.String()

		if _, ok := dimMetaMap[dimKey]; !ok {
			slug := ""
			if v.DimensionSlug != nil {
				slug = *v.DimensionSlug
			}

			sourceKey := ""
			if v.SourceFieldKey != nil {
				sourceKey = *v.SourceFieldKey
			}

			dimMetaMap[dimKey] = dimMeta{Label: v.DimensionLabel, Key: dimKey, Slug: slug, SourceKey: sourceKey, Money: v.DimensionType == dimensionTypeCurrency}
			dimOrder = append(dimOrder, dimKey)
		}

		if dimValues[dimKey] == nil {
			dimValues[dimKey] = make(map[string]float64)
		}

		if cityRawValues[cityKey] == nil {
			cityRawValues[cityKey] = make(map[string]string)
		}

		rawStr := ""
		if v.Value != nil {
			rawStr = *v.Value
		}

		cityRawValues[cityKey][dimKey] = rawStr

		if f, ok := parseFloatValue(v.Value, v.ValueRating); ok {
			dimValues[dimKey][cityKey] = f
		}
	}

	// Build the Cities slice (baseline first, then requested in order).
	cityOrder := make([]pgtype.UUID, 0, len(allIDs))
	cityOrder = append(cityOrder, baselineID)

	for _, id := range cityIDs {
		if id.Valid && id != baselineID {
			cityOrder = append(cityOrder, id)
		}
	}

	// Batch-fetch all non-baseline cities in one query (avoids N+1).
	nonBaselineIDs := make([]pgtype.UUID, 0, len(cityOrder))

	for _, id := range cityOrder {
		if id != baselineID {
			nonBaselineIDs = append(nonBaselineIDs, id)
		}
	}

	cityRows := make(map[pgtype.UUID]gen.GetCOLCitiesByIDsRow, len(nonBaselineIDs))

	if len(nonBaselineIDs) > 0 {
		rows, err := s.store.GetCitiesByIDs(ctx, nonBaselineIDs)
		if err != nil {
			slog.Warn("costofliving: compare: batch get cities", "error", err)
		} else {
			for _, r := range rows {
				cityRows[r.ID] = r
			}
		}
	}

	// Overlay community field values from each city's linked geo space
	// (entity_type='space') — cfv keys matched by dimension slug shadow the
	// stored comparison_values so approved wiki edits reach the calculator
	// (§80 Tool Surface Hydration). A dimension bound to a wiki field via
	// source_field_id resolves its cfv by that field's key instead — the
	// wiki city's Cost of Living page stores community values under the
	// field group's keys, not the dimension slugs. comparison_values rows
	// themselves are never community-edited. One batched cfv read per
	// comparison.
	geoSpaceByCity := make(map[pgtype.UUID]pgtype.UUID, len(cityOrder))
	if baseline.GeoSpaceID.Valid {
		geoSpaceByCity[baselineID] = baseline.GeoSpaceID
	}

	for id, c := range cityRows {
		if c.GeoSpaceID.Valid {
			geoSpaceByCity[id] = c.GeoSpaceID
		}
	}

	geoSpaceIDs := make([]pgtype.UUID, 0, len(geoSpaceByCity))
	for _, spaceID := range geoSpaceByCity {
		geoSpaceIDs = append(geoSpaceIDs, spaceID)
	}

	spaceCFV := s.store.ListSpaceFieldValues(ctx, geoSpaceIDs)

	for cityID, spaceID := range geoSpaceByCity {
		cfv := spaceCFV[spaceID]
		if len(cfv) == 0 {
			continue
		}

		cityKey := cityID.String()

		for dimKey, meta := range dimMetaMap {
			v, ok := cfv[meta.Slug]

			if meta.SourceKey != "" {
				if sv, sok := cfv[meta.SourceKey]; sok {
					v, ok = sv, true
				}
			}

			if !ok {
				continue
			}

			if dimValues[dimKey] == nil {
				dimValues[dimKey] = make(map[string]float64)
			}

			if f, ok := anyToFloat(v); ok {
				dimValues[dimKey][cityKey] = f
			}

			if cityRawValues[cityKey] == nil {
				cityRawValues[cityKey] = make(map[string]string)
			}

			cityRawValues[cityKey][dimKey] = cfvDisplayString(v)
		}
	}

	// §58+§112 normalization: money values are denominated in each city's
	// country currency — convert to the fx display base so ratios and the
	// index compare like units. Cities without a resolvable currency or
	// matrix coverage keep raw local values flagged converted:false.
	spaceCurrencies := s.store.SpaceCurrencyCodes(ctx, geoSpaceIDs)
	cityCurrency := make(map[string]string, len(cityOrder))
	ratesByCity := make(map[string]float64, len(cityOrder))
	ratesByCode := make(map[string]float64)
	displayCurrency := ""
	fxFetchedAt := ""
	fxStale := false

	for _, id := range cityOrder {
		spaceID := geoSpaceByCity[id]

		code := spaceCurrencies[spaceID]
		if code != "" {
			cityCurrency[id.String()] = code
		}

		if s.fxRates == nil || code == "" {
			continue
		}

		if _, ok := ratesByCode[code]; !ok {
			rate, base, fetchedAt, stale, rerr := s.fxRates.BaseRate(ctx, code)
			if rerr != nil {
				continue // unsupported/absent quote → city stays raw
			}

			ratesByCode[code] = rate

			if displayCurrency == "" {
				displayCurrency, fxStale = base, stale
				fxFetchedAt = fetchedAt.UTC().Format(time.RFC3339)
			}
		}

		ratesByCity[id.String()] = ratesByCode[code]
	}

	if displayCurrency != "" {
		for dimID, byCity := range dimValues {
			if !dimMetaMap[dimID].Money {
				continue
			}

			for cityKey, v := range byCity {
				if rate, ok := ratesByCity[cityKey]; ok && rate > 0 {
					byCity[cityKey] = v / rate
				}
			}
		}
	}

	cities := make([]CityData, 0, len(cityOrder))

	for _, id := range cityOrder {
		var title, geoName, geoSlug string

		var lastUpdated time.Time

		if id == baselineID {
			title = baseline.Title

			if baseline.GeoSpaceName != nil {
				geoName = *baseline.GeoSpaceName
			}

			if baseline.GeoSpaceSlug != nil {
				geoSlug = *baseline.GeoSpaceSlug
			}

			if baseline.LastUpdatedAt.Valid {
				lastUpdated = baseline.LastUpdatedAt.Time
			}
		} else {
			c, ok := cityRows[id]
			if !ok {
				slog.Warn("costofliving: compare: city not found", "id", id.String())
				continue
			}

			title = c.Title

			if c.GeoSpaceName != nil {
				geoName = *c.GeoSpaceName
			}

			if c.GeoSpaceSlug != nil {
				geoSlug = *c.GeoSpaceSlug
			}

			if c.LastUpdatedAt.Valid {
				lastUpdated = c.LastUpdatedAt.Time
			}
		}

		lastUpdatedStr := ""
		if !lastUpdated.IsZero() {
			lastUpdatedStr = lastUpdated.UTC().Format(time.RFC3339)
		}
		// Ensure Values is never nil so JSON serializes as {} not null.
		rawVals := cityRawValues[id.String()]
		if rawVals == nil {
			rawVals = map[string]string{}
		}

		cities = append(cities, CityData{
			ID:           id.String(),
			Title:        title,
			GeoSpaceName: geoName,
			GeoSpaceSlug: geoSlug,
			IsBaseline:   id == baselineID,
			LastUpdated:  lastUpdatedStr,
			Values:       rawVals,
			CurrencyCode: cityCurrency[id.String()],
			Converted:    ratesByCity[id.String()] > 0,
		})
	}

	// Step 5: compute ratios vs baseline for each category.
	baselineKey := baselineID.String()
	categories := make([]CategoryData, 0, len(dimOrder))
	overall := make(map[string]float64)

	for _, dimKey := range dimOrder {
		meta := dimMetaMap[dimKey]
		cat := CategoryData{
			Key:    meta.Key,
			Label:  meta.Label,
			Values: make(map[string]float64),
			Ratios: make(map[string]float64),
		}
		baseVal, hasBase := dimValues[dimKey][baselineKey]

		for _, id := range cityOrder {
			cityKey := id.String()

			val, ok := dimValues[dimKey][cityKey]
			if ok {
				cat.Values[cityKey] = val
			}

			// In fx mode, a ratio between converted and unconverted
			// (raw-local) values would silently mix units — skip it.
			mixedUnits := displayCurrency != "" && dimMetaMap[dimKey].Money &&
				(ratesByCity[cityKey] <= 0 || ratesByCity[baselineKey] <= 0)

			if mixedUnits {
				continue
			}

			if hasBase && baseVal > 0 && ok {
				cat.Ratios[cityKey] = val / baseVal
			} else if hasBase && baseVal == 0 && ok {
				// Baseline is zero — ratio undefined; report 0 to avoid NaN.
				cat.Ratios[cityKey] = 0
			}
		}

		categories = append(categories, cat)

		// Step 6: accumulate weighted Overall Cost Index.
		// Weight is looked up by dimension slug in the config weights
		// (config JSONB weights are keyed by slug, e.g. "rent_1br_city_centre").
		weight, hasWeight := weights[meta.Slug]
		if !hasWeight || weight <= 0 {
			continue
		}

		for cityKey, ratio := range cat.Ratios {
			overall[cityKey] += ratio * weight
		}
	}

	return ComparisonMatrix{
		Cities:          cities,
		Categories:      categories,
		BaselineCityID:  baselineKey,
		OverallIndex:    overall,
		DisplayCurrency: displayCurrency,
		FxFetchedAt:     fxFetchedAt,
		FxStale:         fxStale,
	}, nil
}

// AdminStats returns the data completeness + staleness report for admin.
// Gated by col_calculator.manage.
func (s *Service) AdminStats(ctx context.Context, page, limit int) (AdminStatsResponse, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := int32((page - 1) * limit) // #nosec

	typeDef, err := s.store.GetCostOfLivingType(ctx)
	if err != nil {
		return AdminStatsResponse{}, fmt.Errorf("costofliving: admin stats: %w", err)
	}

	rows, err := s.store.AdminStats(ctx, typeDef.ID, offset, int32(limit)) // #nosec
	if err != nil {
		return AdminStatsResponse{}, err
	}

	items := make([]AdminStatsItem, 0, len(rows))

	for _, r := range rows {
		lastUpdated := ""

		var lastUpdatedT time.Time
		if r.LastUpdatedAt.Valid {
			lastUpdatedT = r.LastUpdatedAt.Time
			lastUpdated = lastUpdatedT.UTC().Format(time.RFC3339)
		}

		stale := false
		if !lastUpdatedT.IsZero() {
			stale = s.IsStale(ctx, lastUpdatedT)
		}

		pct := 0.0
		if r.TotalDimensions > 0 {
			pct = float64(r.FilledDimensions) / float64(r.TotalDimensions) * 100
		}

		items = append(items, AdminStatsItem{
			CityID:           r.CityID.String(),
			CityName:         r.CityName,
			IsBaseline:       r.IsBaseline,
			LastUpdated:      lastUpdated,
			IsStale:          stale,
			TotalDimensions:  r.TotalDimensions,
			FilledDimensions: r.FilledDimensions,
			CompletenessPct:  pct,
		})
	}

	// Total count of cities for pagination metadata.
	total, err := s.store.CountCities(ctx, typeDef.ID, "", "", pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		return AdminStatsResponse{}, err
	}

	return AdminStatsResponse{
		Cities: items,
		Total:  total,
		Page:   page,
		Limit:  limit,
	}, nil
}

// parseWeights extracts the per-category weights from the comparison type
// definition's config JSONB. The config shape is:
//
//	{"weights": {"<dimension_slug>": 0.25, ...}}
//
// Weight keys are dimension slugs (e.g. "rent_1br_city_centre"), matching
// the slug column on comparison_dimensions. Returns an empty map if config
// is nil or malformed.
func parseWeights(config []byte) map[string]float64 {
	if len(config) == 0 {
		return map[string]float64{}
	}

	var wrapper struct {
		Weights map[string]float64 `json:"weights"`
	}

	if err := json.Unmarshal(config, &wrapper); err != nil {
		slog.Warn("costofliving: parse weights: invalid config json", "error", err)
		return map[string]float64{}
	}

	if wrapper.Weights == nil {
		return map[string]float64{}
	}

	return wrapper.Weights
}

// anyToFloat coerces a display-ready custom-field value (float64/string/etc.)
// into a float for the comparison matrix, applying the same currency-prefix
// cleanup as parseFloatValue for string values. price_range values arrive as
// {"avg","min","max"} maps — the avg part wins, then the min/max midpoint.
func anyToFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		return parseFloatValue(&n, pgtype.Numeric{})
	case map[string]any:
		return priceRangeToFloat(n)
	}

	return 0, false
}

// priceRangeToFloat reduces a price_range {"avg","min","max"} value to a
// single number: "avg" when present, else the midpoint of min and max,
// else whichever bound exists (§80 Hierarchical UX — price_range carries
// its comparable value in the avg part).
func priceRangeToFloat(m map[string]any) (float64, bool) {
	num := func(key string) (float64, bool) {
		raw, ok := m[key]
		if !ok {
			return 0, false
		}

		switch n := raw.(type) {
		case float64:
			return n, true
		case float32:
			return float64(n), true
		case int:
			return float64(n), true
		case int32:
			return float64(n), true
		case int64:
			return float64(n), true
		case string:
			return parseFloatValue(&n, pgtype.Numeric{})
		}

		return 0, false
	}

	if avg, ok := num("avg"); ok {
		return avg, true
	}

	min, hasMin := num("min")
	max, hasMax := num("max")

	switch {
	case hasMin && hasMax:
		return (min + max) / 2, true
	case hasMin:
		return min, true
	case hasMax:
		return max, true
	}

	return 0, false
}

// cfvDisplayString renders a display-ready custom-field value as the raw
// string shown in CityData.Values. Scalars format naturally; structured
// values (lists/maps) serialise as JSON rather than Go-syntax dumps.
func cfvDisplayString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(t), 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case map[string]any:
		// price_range {"avg","min","max"} — render as a compact range.
		return priceRangeDisplayString(t, v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}

		return string(b)
	}
}

// priceRangeDisplayString renders a price_range map for the raw-value
// surface: "avg" alone when present, else "min–max".
func priceRangeDisplayString(m map[string]any, fallback any) string {
	render := func(key string) string {
		if raw, ok := m[key]; ok {
			return cfvDisplayString(raw)
		}

		return ""
	}

	avg, min, max := render("avg"), render("min"), render("max")

	switch {
	case avg != "" && (min != "" || max != ""):
		return fmt.Sprintf("%s (%s–%s)", avg, min, max)
	case avg != "":
		return avg
	case min != "" && max != "":
		return min + "–" + max
	case min != "":
		return min
	case max != "":
		return max
	}

	return fmt.Sprint(fallback)
}

// parseFloatValue parses a numeric cost value from the comparison_values row.
// Cost data is stored as text (value field) per Section 72; value_rating is
// the numeric variant. Returns the float64 and true if a number was found.
func parseFloatValue(textVal *string, numericVal pgtype.Numeric) (float64, bool) {
	if textVal != nil && *textVal != "" {
		s := strings.TrimSpace(*textVal)
		// Strip common currency prefixes/suffixes before parsing.
		s = strings.TrimPrefix(s, "$")
		s = strings.TrimPrefix(s, "USD")
		s = strings.TrimSpace(s)

		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f, true
		}
	}
	// Fall back to the numeric column if the text parse failed.
	if numericVal.Valid {
		if f, err := numericVal.Float64Value(); err == nil {
			return f.Float64, true
		}
	}

	return 0, false
}
