-- 000687_intel_phase2_settings.up.sql
-- Seed the intel-channels Phase 2 setting that bounds manifest fan-out
-- (intel-channels curation spec §3 sitemap/wayback derived fetches).
-- intel.sitemap.max_children_per_crawl caps the number of derived
-- child fetches a changed manifest may enqueue per crawl; the worker
-- falls back to the same default when the row is absent, so this seed
-- exists to keep the key admin-editable from the control settings UI.
--
-- Post-de-tenant convention: rows are keyed globally on `key`.

BEGIN;

INSERT INTO system_settings (key, value) VALUES
    ('intel.sitemap.max_children_per_crawl', to_jsonb(25))
ON CONFLICT (key) DO NOTHING;

COMMIT;
