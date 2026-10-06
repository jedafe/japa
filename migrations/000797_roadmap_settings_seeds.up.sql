-- 000797_roadmap_settings_seeds.up.sql
-- Seed the §113 roadmap system_settings so the keys are admin-editable from
-- the settings UI (per the system-settings-catalog convention: keys with
-- defaults must be seeded). Code falls back to these same defaults via
-- settingsBool/settingsInt when the rows are absent.
--
-- roadmap.submissions_enabled and roadmap.custom_domain are is_public so
-- GET /site/settings/public exposes them to apps/main — the submit page
-- gates on the former and hooks.server.ts host detection reads the latter.
--
-- Ref: .jeos/sections/113-public-product-roadmap.md
-- Ref: .jeos/conventions/system-settings-catalog.md

BEGIN;

INSERT INTO system_settings (key, value, is_public) VALUES
    ('roadmap.submissions_enabled', to_jsonb(true), true),
    ('roadmap.custom_domain', to_jsonb(''::text), true),
    ('roadmap.board_cache_ttl_seconds', to_jsonb(60), false),
    ('roadmap.shipped_window_days', to_jsonb(90), false)
ON CONFLICT (key) DO NOTHING;

COMMIT;
