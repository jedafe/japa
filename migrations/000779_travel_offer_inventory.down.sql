BEGIN;

DELETE FROM system_settings WHERE key IN (
    'travel.inventory_enabled',
    'travel.max_provider_calls_per_day',
    'travel.offer_ttl_lt7d_seconds',
    'travel.offer_ttl_7_30d_seconds',
    'travel.offer_ttl_gt30d_seconds',
    'travel.offer_ttl_hotel_seconds'
);

DROP TABLE IF EXISTS travel_search_coverage;
DROP TABLE IF EXISTS travel_hotel_offers;
DROP TABLE IF EXISTS travel_flight_offers;

COMMIT;
