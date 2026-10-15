-- 000867_origin_markets_seed.up.sql
-- geo_personalization.origin_markets was never persisted (the archive-era
-- seed did not reach the baseline), so the archetype resolver has been
-- running on its compiled fallback. Seed it now — Nigeria as the single
-- launch origin market — marked public so frontends can pin it in country
-- pickers via GET /site/settings/public (ADR-064).

BEGIN;

INSERT INTO system_settings (key, value, is_public)
VALUES ('geo_personalization.origin_markets', '"NG"', true)
ON CONFLICT (key) DO UPDATE
    SET is_public = true,
        updated_at = now();

COMMIT;
