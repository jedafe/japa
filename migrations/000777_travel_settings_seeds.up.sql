-- 000777_travel_settings_seeds.up.sql
-- Seed the travel search settings introduced by the §67 stored-results
-- amendment. Code falls back to these same defaults via GetIntOr when the
-- rows are absent; the seeds exist so the keys are admin-editable from the
-- settings UI (per the system-settings-catalog convention: keys with
-- defaults must be seeded).
--
-- Ref: .jeos/conventions/system-settings-catalog.md
--      .jeos/sections/67-travel-search.md

BEGIN;

INSERT INTO system_settings (key, value) VALUES
    ('travel.results_ttl_seconds', to_jsonb(3600)),
    ('travel.results_retention_days', to_jsonb(30))
ON CONFLICT (key) DO NOTHING;

COMMIT;
