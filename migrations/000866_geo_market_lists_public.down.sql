-- 000866_geo_market_lists_public.down.sql
-- Revert the public flag on the geo market list settings.

BEGIN;

UPDATE system_settings
SET is_public = false,
    updated_at = now()
WHERE key IN (
    'geo_personalization.origin_markets',
    'geo_personalization.destination_markets'
);

COMMIT;
