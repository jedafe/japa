-- 000779_travel_offer_inventory.up.sql
-- ADR-124: offer-keyed travel inventory + coverage bookkeeping.
--
-- The existing travel_search_results table is a REPLAY cache — it stores a
-- whole provider response keyed by the exact canonical query. Every distinct
-- param shape (sort order, filter, page, pax mix) costs one upstream call.
-- These tables invert that: provider responses decompose into per-offer rows
-- indexed by route/date/pax coverage dimensions, so one upstream call serves
-- many query shapes (sort/filter/page applied at serve time).
--
-- Ref: .jeos/adr/ADR-124-travel-offer-inventory.md
--      .jeos/sections/67-travel-search.md

BEGIN;

CREATE TABLE travel_flight_offers (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id   uuid NOT NULL REFERENCES travel_providers(id) ON DELETE CASCADE,
    route_key     text NOT NULL,          -- 'LOS-LHR' (upper)
    depart_date   date NOT NULL,
    return_date   date,                   -- NULL = one-way
    adults        smallint NOT NULL,
    children      smallint NOT NULL,
    cabin_class   text NOT NULL DEFAULT '',
    offer_key     text NOT NULL,          -- coverage-scoped: coverageKey|id:{offer_id} or coverageKey|h:{content hash}
    price         numeric(12,2) NOT NULL,
    currency      char(3) NOT NULL DEFAULT '',
    airline       text,
    flight_number text,
    stops         smallint,
    depart_ts     timestamptz,
    arrive_ts     timestamptz,
    duration      text,
    booking_url   text NOT NULL,          -- canonical URL, affiliate-free (ADR-009)
    raw           jsonb,
    fetched_at    timestamptz NOT NULL DEFAULT now(),
    expires_at    timestamptz NOT NULL,
    UNIQUE (provider_id, offer_key)
);

CREATE TABLE travel_hotel_offers (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id   uuid NOT NULL REFERENCES travel_providers(id) ON DELETE CASCADE,
    city_key      text NOT NULL,          -- lowercase trimmed destination token
    checkin       date NOT NULL,
    checkout      date NOT NULL,
    guests        smallint NOT NULL,
    rooms         smallint NOT NULL,
    offer_key     text NOT NULL,
    name          text NOT NULL,
    price         numeric(12,2) NOT NULL,
    currency      char(3) NOT NULL DEFAULT '',
    star_rating   smallint,
    address       text,
    image_url     text,
    booking_url   text NOT NULL,
    raw           jsonb,
    fetched_at    timestamptz NOT NULL DEFAULT now(),
    expires_at    timestamptz NOT NULL,
    UNIQUE (provider_id, offer_key)
);

-- Coverage bookkeeping: which coverage spaces have been fetched, per
-- provider. The coverage_key EXCLUDES presentation params (sort/filter/page)
-- so every presentation shape of a covered query serves from inventory.
CREATE TABLE travel_search_coverage (
    search_type  text NOT NULL CHECK (search_type IN ('flight','hotel')),
    coverage_key text NOT NULL,
    provider_id  uuid NOT NULL REFERENCES travel_providers(id) ON DELETE CASCADE,
    fetched_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL,
    offer_count  int NOT NULL DEFAULT 0,
    PRIMARY KEY (search_type, coverage_key, provider_id)
);

-- Coverage-dimension lookups (the hot read path).
CREATE INDEX idx_flight_offers_coverage ON travel_flight_offers
    (provider_id, route_key, depart_date, adults, children, cabin_class)
    INCLUDE (price, airline, stops, depart_ts, expires_at);
CREATE INDEX idx_hotel_offers_coverage ON travel_hotel_offers
    (provider_id, city_key, checkin, checkout, guests, rooms)
    INCLUDE (price, star_rating, expires_at);

-- Retention sweeps.
CREATE INDEX idx_flight_offers_expiry ON travel_flight_offers (expires_at);
CREATE INDEX idx_hotel_offers_expiry  ON travel_hotel_offers (expires_at);
CREATE INDEX idx_travel_coverage_expiry ON travel_search_coverage (expires_at);

-- Settings (§67). inventory_enabled defaults OFF — rollout flag; flip after
-- verification. Quota ceiling defaults to a conservative affiliate-plan
-- allowance; <= 0 disables.
INSERT INTO system_settings (key, value) VALUES
    ('travel.inventory_enabled',           to_jsonb(false)),
    ('travel.max_provider_calls_per_day',  to_jsonb(1000)),
    ('travel.offer_ttl_lt7d_seconds',      to_jsonb(1800)),
    ('travel.offer_ttl_7_30d_seconds',     to_jsonb(7200)),
    ('travel.offer_ttl_gt30d_seconds',     to_jsonb(21600)),
    ('travel.offer_ttl_hotel_seconds',     to_jsonb(21600))
ON CONFLICT (key) DO NOTHING;

COMMIT;
