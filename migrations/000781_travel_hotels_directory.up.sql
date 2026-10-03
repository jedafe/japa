-- §67, ADR-124 — hotel directory reference table.
-- Persistent hotel PROPERTIES (entity identity, not coverage-scoped):
-- upserted from every provider hotel search and from the
-- travel:sync_hotel_directory task (provider extra.hotels_endpoint feed).
-- last_price/last_currency denormalize the cheapest last-seen offer so the
-- directory survives offer expiry/pruning.
CREATE TABLE IF NOT EXISTS travel_hotels (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id   uuid NOT NULL REFERENCES travel_providers(id) ON DELETE CASCADE,
    hotel_key     text NOT NULL,          -- id:{provider hotel_id/offer_id} or h:{sha256(name|address|city_key)}
    city_key      text NOT NULL,          -- lowercase trimmed destination token
    name          text NOT NULL,
    star_rating   smallint,
    address       text,
    image_url     text,
    last_price    numeric(12,2),
    last_currency char(3),
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_id, hotel_key)
);

CREATE INDEX IF NOT EXISTS idx_travel_hotels_city ON travel_hotels (city_key, name);
CREATE INDEX IF NOT EXISTS idx_travel_hotels_seen ON travel_hotels (last_seen_at DESC);
