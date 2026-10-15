-- 000867_origin_markets_seed.down.sql
-- Revert the public flag on origin_markets (leave the row — removing it
-- would just restore the compiled fallback).

BEGIN;

UPDATE system_settings
SET is_public = false,
    updated_at = now()
WHERE key = 'geo_personalization.origin_markets';

COMMIT;
