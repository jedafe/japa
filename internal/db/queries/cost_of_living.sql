-- cost_of_living.sql — Cost of Living Calculator queries (Section 72, ADR-072).
-- Queries the existing §30 comparison system for cost_of_living type data.
-- Ref: japaconnect-engineering/sections/72-cost-of-living.md
--      japaconnect-engineering/adr/ADR-072-cost-of-living-calculator.md

-- =====================================================================
-- Get the cost_of_living comparison type definition (with config JSONB
-- containing the weighted cost index weights).
-- =====================================================================

-- name: GetCostOfLivingType :one
SELECT * FROM comparison_type_definitions
WHERE slug = 'cost_of_living' AND is_system = true;

-- =====================================================================
-- List cities (comparison_items) with cost_of_living data, paginated +
-- searchable by title. Joins geo spaces for country/region context.
-- =====================================================================

-- name: ListCOLCities :many
SELECT
 ci.id,
 ci.title,
 ci.description,
 ci.geo_space_id,
 ci.is_baseline,
 ci.last_updated_at,
 ci.position,
 ci.intel_cost_data,
 s.name AS geo_space_name,
 s.slug AS geo_space_slug,
 -- Per-city currency for local display (§72/§58): city spaces rarely carry
 -- their own currency_code, so fall back up the state→country hierarchy.
 COALESCE(s.currency_code, state.currency_code, country.currency_code) AS currency_code
FROM comparison_items ci
LEFT JOIN spaces s ON s.id = ci.geo_space_id AND s.deleted_at IS NULL
LEFT JOIN space_parent_spaces sps_state ON sps_state.space_id = s.id
LEFT JOIN spaces state ON state.id = sps_state.parent_space_id AND state.level = 'state' AND state.deleted_at IS NULL
LEFT JOIN space_parent_spaces sps_country ON sps_country.space_id = state.id
LEFT JOIN spaces country ON country.id = sps_country.parent_space_id AND country.level = 'country' AND country.deleted_at IS NULL
WHERE ci.type_id = @type_id
 AND ci.deleted_at IS NULL
 AND (@search::text = '' OR ci.title ILIKE '%' || @search || '%')
 AND (@country_code::text = '' OR country.geo_code = @country_code)
 AND (@country_id::uuid IS NULL OR country.id = @country_id)
 AND (@state_id::uuid IS NULL OR state.id = @state_id)
 AND (@city_id::uuid IS NULL OR s.id = @city_id)
ORDER BY ci.is_baseline DESC, ci.title ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountCOLCities :one
SELECT COUNT(*) AS count
FROM comparison_items ci
LEFT JOIN spaces s ON s.id = ci.geo_space_id AND s.deleted_at IS NULL
LEFT JOIN space_parent_spaces sps_state ON sps_state.space_id = s.id
LEFT JOIN spaces state ON state.id = sps_state.parent_space_id AND state.level = 'state' AND state.deleted_at IS NULL
LEFT JOIN space_parent_spaces sps_country ON sps_country.space_id = state.id
LEFT JOIN spaces country ON country.id = sps_country.parent_space_id AND country.level = 'country' AND country.deleted_at IS NULL
WHERE ci.type_id = @type_id
 AND ci.deleted_at IS NULL
 AND (@search::text = '' OR ci.title ILIKE '%' || @search || '%')
 AND (@country_code::text = '' OR country.geo_code = @country_code)
 AND (@country_id::uuid IS NULL OR country.id = @country_id)
 AND (@state_id::uuid IS NULL OR state.id = @state_id)
 AND (@city_id::uuid IS NULL OR s.id = @city_id);

-- =====================================================================
-- Get a single city (comparison_item) by ID with its geo space info.
-- =====================================================================

-- name: GetCOLCityByID :one
SELECT
 ci.*,
 s.name AS geo_space_name,
 s.slug AS geo_space_slug,
 COALESCE(s.currency_code, state.currency_code, country.currency_code) AS currency_code
FROM comparison_items ci
LEFT JOIN spaces s ON s.id = ci.geo_space_id
LEFT JOIN space_parent_spaces sps_state ON sps_state.space_id = s.id
LEFT JOIN spaces state ON state.id = sps_state.parent_space_id AND state.level = 'state' AND state.deleted_at IS NULL
LEFT JOIN space_parent_spaces sps_country ON sps_country.space_id = state.id
LEFT JOIN spaces country ON country.id = sps_country.parent_space_id AND country.level = 'country' AND country.deleted_at IS NULL
WHERE ci.id = @id AND ci.deleted_at IS NULL;

-- =====================================================================
-- Batch-fetch multiple cities by ID (removes N+1 in CompareCities).
-- Returns the same columns as GetCOLCityByID for a set of item IDs.
-- =====================================================================

-- name: GetCOLCitiesByIDs :many
SELECT
 ci.id,
 ci.title,
 ci.description,
 ci.geo_space_id,
 ci.is_baseline,
 ci.last_updated_at,
 ci.position,
 s.name AS geo_space_name,
 s.slug AS geo_space_slug,
 COALESCE(s.currency_code, state.currency_code, country.currency_code) AS currency_code
FROM comparison_items ci
LEFT JOIN spaces s ON s.id = ci.geo_space_id
LEFT JOIN space_parent_spaces sps_state ON sps_state.space_id = s.id
LEFT JOIN spaces state ON state.id = sps_state.parent_space_id AND state.level = 'state' AND state.deleted_at IS NULL
LEFT JOIN space_parent_spaces sps_country ON sps_country.space_id = state.id
LEFT JOIN spaces country ON country.id = sps_country.parent_space_id AND country.level = 'country' AND country.deleted_at IS NULL
WHERE ci.id = ANY(@ids::uuid[])
 AND ci.deleted_at IS NULL;

-- =====================================================================
-- Get the baseline city for the cost_of_living type.
-- =====================================================================

-- name: GetCOLBaselineCity :one
SELECT
 ci.*,
 s.name AS geo_space_name,
 s.slug AS geo_space_slug,
 COALESCE(s.currency_code, state.currency_code, country.currency_code) AS currency_code
FROM comparison_items ci
LEFT JOIN spaces s ON s.id = ci.geo_space_id
LEFT JOIN space_parent_spaces sps_state ON sps_state.space_id = s.id
LEFT JOIN spaces state ON state.id = sps_state.parent_space_id AND state.level = 'state' AND state.deleted_at IS NULL
LEFT JOIN space_parent_spaces sps_country ON sps_country.space_id = state.id
LEFT JOIN spaces country ON country.id = sps_country.parent_space_id AND country.level = 'country' AND country.deleted_at IS NULL
WHERE ci.type_id = @type_id
 AND ci.is_baseline = true
 AND ci.deleted_at IS NULL
LIMIT 1;

-- =====================================================================
-- Get comparison values for a set of city items (the cost data matrix).
-- Returns dimension_id, item_id, and the value fields for all specified
-- city IDs. The service layer pivots these into a matrix.
-- =====================================================================

-- name: GetCOLValuesForCities :many
SELECT
 cv.dimension_id,
 cv.item_id,
 cv.value,
 cv.value_rating,
 cv.value_boolean,
 cd.label AS dimension_label,
 cd.slug AS dimension_slug,
 cd.dimension_type,
 cf.key AS source_field_key
FROM comparison_values cv
JOIN comparison_dimensions cd ON cd.id = cv.dimension_id AND cd.deleted_at IS NULL
LEFT JOIN custom_fields cf ON cf.id = cd.source_field_id
WHERE cv.item_id = ANY(@item_ids::uuid[])
ORDER BY cd.position, cv.item_id;

-- =====================================================================
-- Admin: data completeness + staleness report per city.
-- Returns each city with: total dimensions, filled dimensions, and
-- whether data is stale (last_updated_at older than threshold).
-- =====================================================================

-- name: AdminCOLStats :many
SELECT
 ci.id AS city_id,
 ci.title AS city_name,
 ci.is_baseline,
 ci.last_updated_at,
 COUNT(DISTINCT cd.id) AS total_dimensions,
 COUNT(DISTINCT CASE WHEN cv.value IS NOT NULL AND cv.value != '' THEN cd.id END) AS filled_dimensions
FROM comparison_items ci
LEFT JOIN comparison_dimensions cd ON cd.content_id = ci.content_id AND cd.deleted_at IS NULL
LEFT JOIN comparison_values cv ON cv.dimension_id = cd.id AND cv.item_id = ci.id
WHERE ci.type_id = @type_id AND ci.deleted_at IS NULL
GROUP BY ci.id, ci.title, ci.is_baseline, ci.last_updated_at
ORDER BY ci.is_baseline DESC, ci.title ASC
LIMIT @page_limit OFFSET @page_offset;

-- =====================================================================
-- Get a single cost_of-living item by its associated geo space.
-- Used by the JCE resolver to populate real score_inputs for spaces.
-- =====================================================================

-- name: GetCOLByGeoSpaceID :one
SELECT
 ci.*,
 s.name AS geo_space_name,
 s.slug AS geo_space_slug
FROM comparison_items ci
LEFT JOIN spaces s ON s.id = ci.geo_space_id
WHERE ci.type_id = (SELECT id FROM comparison_type_definitions
 WHERE slug = 'cost_of_living'
 AND is_system = true
 LIMIT 1)
 AND ci.geo_space_id = @geo_space_id
 AND ci.deleted_at IS NULL
LIMIT 1;
