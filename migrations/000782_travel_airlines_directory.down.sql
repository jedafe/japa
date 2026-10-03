BEGIN;
ALTER TABLE travel_flight_offers DROP COLUMN IF EXISTS airline_key;
DROP TABLE IF EXISTS travel_provider_airlines;
DROP TABLE IF EXISTS travel_airlines;
COMMIT;
