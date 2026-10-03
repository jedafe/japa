-- travel.sql — Travel Search & Compare queries (Section 67, ADR-085).
-- Covers travel provider CRUD and admin oversight.
-- Ref: japaconnect-engineering/sections/67-travel-search.md
--      japaconnect-engineering/adr/ADR-085-travel-search.md

-- =====================================================================
-- Create a travel provider.
-- =====================================================================

-- name: CreateTravelProvider :one
INSERT INTO travel_providers (name, slug, provider_type, api_config, affiliate_config, is_active) VALUES (@name, @slug, @provider_type, sqlc.narg(api_config), sqlc.narg(affiliate_config), @is_active)

RETURNING *;

-- =====================================================================
-- Update a travel provider.
-- =====================================================================

-- name: UpdateTravelProvider :one
UPDATE travel_providers
SET
 name = COALESCE(sqlc.narg(name), name),
 provider_type = COALESCE(sqlc.narg(provider_type), provider_type),
 api_config = COALESCE(sqlc.narg(api_config), api_config),
 affiliate_config = COALESCE(sqlc.narg(affiliate_config), affiliate_config),
 is_active = COALESCE(sqlc.narg(is_active), is_active)
WHERE id = @id
RETURNING *;

-- =====================================================================
-- Get a single travel provider by ID.
-- =====================================================================

-- name: GetTravelProviderByID :one
SELECT * FROM travel_providers
WHERE id = @id;

-- =====================================================================
-- List travel providers with optional search and active-status filter.
-- q: ILIKE search on name or slug (empty = no filter).
-- is_active_filter: tri-state — 'true' = only active, 'false' = only
--   inactive, '' (empty) = no filter (all statuses).
-- provider_type: filter by exact type (empty = all).
-- =====================================================================

-- name: ListTravelProviders :many
SELECT * FROM travel_providers
WHERE (@is_active_filter::text = '' OR is_active = (@is_active_filter::text = 'true'))
 AND (@provider_type::text = '' OR provider_type = @provider_type)
 AND (@q::text = '' OR name ILIKE '%' || @q::text || '%' OR slug ILIKE '%' || @q::text || '%')
ORDER BY name ASC;

-- =====================================================================
-- Get the active provider for a given type (flight, hotel, or both).
-- Used by the search proxy to find the configured provider.
-- =====================================================================

-- name: GetActiveTravelProvider :one
SELECT * FROM travel_providers
WHERE is_active = true
 AND (provider_type = @provider_type OR provider_type = 'both')
ORDER BY
 CASE WHEN provider_type = @provider_type THEN 0 ELSE 1 END,
 created_at DESC
LIMIT 1;

-- =====================================================================
-- Search the airport catalog for the /travel/locations autocomplete.
-- Exact IATA match ranks first, then IATA prefix, city prefix, contains.
-- =====================================================================

-- name: SearchTravelAirports :many
SELECT iata_code, name, city, country_code FROM travel_airports
WHERE iata_code ILIKE @q::text || '%' ESCAPE '\'
 OR city ILIKE '%' || @q::text || '%' ESCAPE '\'
 OR name ILIKE '%' || @q::text || '%' ESCAPE '\'
ORDER BY
 CASE
  WHEN iata_code = upper(@q) THEN 0
  WHEN iata_code ILIKE @q::text || '%' ESCAPE '\' THEN 1
  WHEN city ILIKE @q::text || '%' ESCAPE '\' THEN 2
  ELSE 3
 END,
 CASE airport_type WHEN 'large' THEN 0 ELSE 1 END,
 city ASC
LIMIT @row_limit;

-- =====================================================================
-- Persisted search results — one row per unique canonical query,
-- refreshed from the provider once expires_at passes (§67).
-- =====================================================================

-- name: GetTravelSearchResult :one
SELECT * FROM travel_search_results
WHERE search_type = @search_type AND query_hash = @query_hash;

-- name: UpsertTravelSearchResult :one
INSERT INTO travel_search_results (search_type, query_hash, params, results, provider_id, expires_at)
VALUES (@search_type, @query_hash, @params, @results, sqlc.narg(provider_id), @expires_at)
ON CONFLICT (search_type, query_hash) DO UPDATE SET
 results = EXCLUDED.results,
 params = EXCLUDED.params,
 provider_id = EXCLUDED.provider_id,
 fetched_at = now(),
 expires_at = EXCLUDED.expires_at,
 hit_count = 0
RETURNING *;

-- name: BumpTravelSearchResultHit :exec
UPDATE travel_search_results SET hit_count = hit_count + 1 WHERE id = @id;

-- =====================================================================
-- Weekly retention sweep for stored search results (maintenance:prune_travel_results).
-- Expired rows are kept solely for stale-fallback; once they are older than
-- travel.results_retention_days (default 30) they are useless and deleted.
-- =====================================================================

-- name: PruneTravelSearchResults :exec
-- Retention via system_settings 'travel.results_retention_days' (default 30).
DELETE FROM travel_search_results r
USING (
 SELECT (COALESCE((SELECT s.value->>0
 FROM system_settings s
 WHERE s.key = 'travel.results_retention_days'),
 '30')::int || ' days')::interval AS retention
) cfg
WHERE r.expires_at < now() - cfg.retention;

-- =====================================================================
-- Offer inventory (ADR-124) — per-offer upserts keyed on
-- (provider_id, offer_key), coverage-scoped reads for the serving path,
-- and coverage bookkeeping.
-- =====================================================================

-- name: UpsertFlightOffer :exec
INSERT INTO travel_flight_offers (
 provider_id, route_key, depart_date, return_date, adults, children,
 cabin_class, offer_key, price, currency, airline, flight_number, stops,
 depart_ts, arrive_ts, duration, booking_url, raw, expires_at, airline_key
) VALUES (
 @provider_id, @route_key, @depart_date, sqlc.narg(return_date), @adults,
 @children, @cabin_class, @offer_key, @price, @currency,
 sqlc.narg(airline), sqlc.narg(flight_number), sqlc.narg(stops),
 sqlc.narg(depart_ts), sqlc.narg(arrive_ts), sqlc.narg(duration),
 @booking_url, sqlc.narg(raw), @expires_at, sqlc.narg(airline_key)
)
ON CONFLICT (provider_id, offer_key) DO UPDATE SET
 price = EXCLUDED.price,
 currency = EXCLUDED.currency,
 airline = EXCLUDED.airline,
 airline_key = EXCLUDED.airline_key,
 flight_number = EXCLUDED.flight_number,
 stops = EXCLUDED.stops,
 depart_ts = EXCLUDED.depart_ts,
 arrive_ts = EXCLUDED.arrive_ts,
 duration = EXCLUDED.duration,
 booking_url = EXCLUDED.booking_url,
 raw = EXCLUDED.raw,
 fetched_at = now(),
 expires_at = EXCLUDED.expires_at;

-- name: UpsertHotelOffer :exec
INSERT INTO travel_hotel_offers (
 provider_id, city_key, checkin, checkout, guests, rooms, offer_key,
 name, price, currency, star_rating, address, image_url, booking_url,
 raw, expires_at
) VALUES (
 @provider_id, @city_key, @checkin, @checkout, @guests, @rooms, @offer_key,
 @name, @price, @currency, sqlc.narg(star_rating), sqlc.narg(address),
 sqlc.narg(image_url), @booking_url, sqlc.narg(raw), @expires_at
)
ON CONFLICT (provider_id, offer_key) DO UPDATE SET
 name = EXCLUDED.name,
 price = EXCLUDED.price,
 currency = EXCLUDED.currency,
 star_rating = EXCLUDED.star_rating,
 address = EXCLUDED.address,
 image_url = EXCLUDED.image_url,
 booking_url = EXCLUDED.booking_url,
 raw = EXCLUDED.raw,
 fetched_at = now(),
 expires_at = EXCLUDED.expires_at;

-- Offers for a flight coverage (provider + route/date/pax/cabin).
-- include_expired=false → fresh-only serving path; true → stale fallback.
-- Presentation (sort/filter/page) happens in the service — see §67.
-- name: ListFlightOffersByCoverage :many
SELECT * FROM travel_flight_offers
WHERE provider_id = @provider_id
 AND route_key = @route_key
 AND depart_date = @depart_date
 AND return_date IS NOT DISTINCT FROM sqlc.narg(return_date)
 AND adults = @adults
 AND children = @children
 AND cabin_class = @cabin_class
 AND (@include_expired::bool OR expires_at > now())
ORDER BY depart_ts ASC NULLS LAST, price ASC;

-- name: ListHotelOffersByCoverage :many
SELECT * FROM travel_hotel_offers
WHERE provider_id = @provider_id
 AND city_key = @city_key
 AND checkin = @checkin
 AND checkout = @checkout
 AND guests = @guests
 AND rooms = @rooms
 AND (@include_expired::bool OR expires_at > now())
ORDER BY price ASC;

-- =====================================================================
-- Coverage bookkeeping — one row per (search_type, coverage_key,
-- provider). Fresh coverage = inventory may serve the query shape.
-- =====================================================================

-- name: GetTravelSearchCoverage :one
SELECT * FROM travel_search_coverage
WHERE search_type = @search_type AND coverage_key = @coverage_key AND provider_id = @provider_id;

-- name: UpsertTravelSearchCoverage :exec
INSERT INTO travel_search_coverage (search_type, coverage_key, provider_id, expires_at, offer_count)
VALUES (@search_type, @coverage_key, @provider_id, @expires_at, @offer_count)
ON CONFLICT (search_type, coverage_key, provider_id) DO UPDATE SET
 fetched_at = now(),
 expires_at = EXCLUDED.expires_at,
 offer_count = EXCLUDED.offer_count;

-- =====================================================================
-- Inventory retention — same window as replay rows (ADR-124 §9).
-- =====================================================================

-- name: PruneTravelFlightOffers :exec
DELETE FROM travel_flight_offers o
USING (
 SELECT (COALESCE((SELECT s.value->>0
 FROM system_settings s
 WHERE s.key = 'travel.results_retention_days'),
 '30')::int || ' days')::interval AS retention
) cfg
WHERE o.expires_at < now() - cfg.retention;

-- name: PruneTravelHotelOffers :exec
DELETE FROM travel_hotel_offers o
USING (
 SELECT (COALESCE((SELECT s.value->>0
 FROM system_settings s
 WHERE s.key = 'travel.results_retention_days'),
 '30')::int || ' days')::interval AS retention
) cfg
WHERE o.expires_at < now() - cfg.retention;

-- Coverage rows that outlive their offers cause no harm — an empty offer
-- list falls through to a provider fetch which re-upserts the row. Prune on
-- the same expiry-plus-retention window.
-- name: PruneTravelSearchCoverage :exec
DELETE FROM travel_search_coverage c
USING (
 SELECT (COALESCE((SELECT s.value->>0
 FROM system_settings s
 WHERE s.key = 'travel.results_retention_days'),
 '30')::int || ' days')::interval AS retention
) cfg
WHERE c.expires_at < now() - cfg.retention;

-- =====================================================================
-- Hotel directory (ADR-124 extension) — travel_hotels reference table.
-- =====================================================================

-- Upsert a directory property. Identity is (provider_id, hotel_key) where
-- hotel_key is the provider's hotel/offer id or a content hash — NOT
-- coverage-scoped: a hotel is the same entity across dates.
-- last_price tracks the cheapest price seen so far for directory display.
-- name: UpsertTravelHotel :exec
INSERT INTO travel_hotels (
 provider_id, hotel_key, city_key, name, star_rating, address, image_url,
 last_price, last_currency
) VALUES (
 @provider_id, @hotel_key, @city_key, @name, sqlc.narg(star_rating),
 sqlc.narg(address), sqlc.narg(image_url), sqlc.narg(last_price),
 sqlc.narg(last_currency)
)
ON CONFLICT (provider_id, hotel_key) DO UPDATE SET
 name = EXCLUDED.name,
 city_key = EXCLUDED.city_key,
 star_rating = COALESCE(EXCLUDED.star_rating, travel_hotels.star_rating),
 address = COALESCE(EXCLUDED.address, travel_hotels.address),
 image_url = COALESCE(EXCLUDED.image_url, travel_hotels.image_url),
 last_price = COALESCE(EXCLUDED.last_price, travel_hotels.last_price),
 last_currency = COALESCE(EXCLUDED.last_currency, travel_hotels.last_currency),
 last_seen_at = now();

-- Directory browse: optional city filter, optional name substring,
-- optional min star rating. Count + page in one round trip.
-- name: ListTravelHotels :many
SELECT * FROM travel_hotels
WHERE (@city_key::text = '' OR city_key = @city_key)
 AND (@name_q::text = '' OR name ILIKE '%' || @name_q || '%' ESCAPE '\')
 AND (@min_stars::int = 0 OR star_rating >= @min_stars)
ORDER BY star_rating DESC NULLS LAST, name ASC, id
LIMIT @lim OFFSET @off;

-- name: CountTravelHotels :one
SELECT count(*) FROM travel_hotels
WHERE (@city_key::text = '' OR city_key = @city_key)
 AND (@name_q::text = '' OR name ILIKE '%' || @name_q || '%' ESCAPE '\')
 AND (@min_stars::int = 0 OR star_rating >= @min_stars);

-- =====================================================================
-- Airline directory (ADR-124 extension) — travel_airlines global catalog
-- + travel_provider_airlines coverage join.
-- =====================================================================

-- Resolve an offer's airline value to a catalog row. Two lookups only —
-- exact IATA code (preferring the 'iata:'-keyed row on real-world code
-- reuse) then case-insensitive name; misses upsert an 'observed' row
-- keyed by code or name slug in the service layer.
-- name: FindTravelAirlineByIATA :one
SELECT * FROM travel_airlines
WHERE iata_code = @iata_code
ORDER BY (airline_key LIKE 'iata:%') DESC, last_seen_at DESC NULLS LAST, airline_key
LIMIT 1;

-- name: FindTravelAirlineByName :one
SELECT * FROM travel_airlines
WHERE lower(name) = lower(@name)
ORDER BY (airline_key LIKE 'iata:%') DESC, airline_key
LIMIT 1;

-- name: UpsertTravelAirline :exec
INSERT INTO travel_airlines (
 airline_key, iata_code, icao_code, name, country, source, last_seen_at
) VALUES (
 @airline_key, sqlc.narg(iata_code), sqlc.narg(icao_code), @name,
 @country, @source, now()
)
ON CONFLICT (airline_key) DO UPDATE SET
 last_seen_at = now(),
 iata_code = COALESCE(travel_airlines.iata_code, EXCLUDED.iata_code),
 icao_code = COALESCE(travel_airlines.icao_code, EXCLUDED.icao_code);

-- name: UpsertTravelProviderAirline :exec
INSERT INTO travel_provider_airlines (provider_id, airline_key)
VALUES (@provider_id, @airline_key)
ON CONFLICT (provider_id, airline_key) DO UPDATE SET
 last_seen_at = now();

-- Directory browse: q matches name / IATA / ICAO / country; provider
-- filters to airlines that provider's offers have been observed carrying.
-- Coverage joins are restricted to ACTIVE providers — a deactivated
-- integration can't sell seats, so its historical coverage is hidden.
-- name: ListTravelAirlines :many
SELECT
 a.airline_key, a.iata_code, a.icao_code, a.name, a.country, a.source,
 (count(p.id) > 0) AS observed,
 COALESCE(array_agg(p.slug ORDER BY p.slug) FILTER (WHERE p.slug IS NOT NULL), '{}')::text[] AS provider_slugs,
 COALESCE(array_agg(p.name ORDER BY p.slug) FILTER (WHERE p.name IS NOT NULL), '{}')::text[] AS provider_names,
 max(pa.last_seen_at) FILTER (WHERE p.id IS NOT NULL)::timestamptz AS coverage_last_seen_at
FROM travel_airlines a
LEFT JOIN travel_provider_airlines pa ON pa.airline_key = a.airline_key
LEFT JOIN travel_providers p ON p.id = pa.provider_id AND p.is_active
WHERE (@q::text = ''
    OR a.name ILIKE '%' || @q || '%' ESCAPE '\'
    OR a.iata_code ILIKE @q ESCAPE '\'
    OR a.icao_code ILIKE @q ESCAPE '\'
    OR a.country ILIKE '%' || @q || '%' ESCAPE '\')
  AND (@country::text = '' OR a.country ILIKE @country ESCAPE '\')
  AND (@provider_slug::text = '' OR EXISTS (
        SELECT 1 FROM travel_provider_airlines pa2
        JOIN travel_providers p2 ON p2.id = pa2.provider_id
        WHERE pa2.airline_key = a.airline_key AND p2.slug = @provider_slug
          AND p2.is_active))
GROUP BY a.airline_key
ORDER BY observed DESC, lower(a.name), a.airline_key
LIMIT @lim OFFSET @off;

-- name: CountTravelAirlines :one
SELECT count(*) FROM travel_airlines a
WHERE (@q::text = ''
    OR a.name ILIKE '%' || @q || '%' ESCAPE '\'
    OR a.iata_code ILIKE @q ESCAPE '\'
    OR a.icao_code ILIKE @q ESCAPE '\'
    OR a.country ILIKE '%' || @q || '%' ESCAPE '\')
  AND (@country::text = '' OR a.country ILIKE @country ESCAPE '\')
  AND (@provider_slug::text = '' OR EXISTS (
        SELECT 1 FROM travel_provider_airlines pa2
        JOIN travel_providers p2 ON p2.id = pa2.provider_id
        WHERE pa2.airline_key = a.airline_key AND p2.slug = @provider_slug
          AND p2.is_active));
