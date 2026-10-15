-- 000866_geo_market_lists_public.up.sql
-- Expose the ADR-064 archetype market lists via GET /site/settings/public so
-- frontends can pin them first in origin/destination country pickers and the
-- ?geo= content filters (curated-markets rollout). The catalog marks both
-- keys Public for new writes; this migration flips is_public on rows that
-- already exist (the settings upsert preserves is_public on conflict).

BEGIN;

UPDATE system_settings
SET is_public = true,
    updated_at = now()
WHERE key IN (
    'geo_personalization.origin_markets',
    'geo_personalization.destination_markets'
);

COMMIT;
