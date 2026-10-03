-- 000778_travel_locations_ttl_seed.up.sql
-- Seed the provider-locations cache TTL introduced by the §67 quota-controls
-- amendment. Code falls back to 86400 via GetIntOr when absent; the seed
-- exists so the key is admin-editable from the settings UI.
--
-- Ref: .jeos/conventions/system-settings-catalog.md
--      .jeos/sections/67-travel-search.md

BEGIN;

INSERT INTO system_settings (key, value) VALUES
    ('travel.locations_cache_ttl_seconds', to_jsonb(86400))
ON CONFLICT (key) DO NOTHING;

COMMIT;
