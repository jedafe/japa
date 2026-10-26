-- 000937_pwa_settings_seed.up.sql
-- Seed the §124 PWA system_settings so the keys are exposed via
-- GET /site/settings/public (is_public = true) and editable under
-- Admin → Settings → Progressive Web App without an initial write.
-- Manifest endpoints fall back to these same defaults when rows are absent.
--
-- Ref: .jeos/sections/124-progressive-web-app.md
-- Ref: .jeos/conventions/system-settings-catalog.md

BEGIN;

INSERT INTO system_settings (key, value, is_public) VALUES
    ('pwa.name', to_jsonb(''::text), true),
    ('pwa.short_name', to_jsonb(''::text), true),
    ('pwa.description', to_jsonb(''::text), true),
    ('pwa.theme_color', to_jsonb(''::text), true),
    ('pwa.background_color', to_jsonb('#ffffff'::text), true),
    ('pwa.display', to_jsonb('standalone'::text), true),
    ('pwa.start_url', to_jsonb('/'::text), true),
    ('pwa.icon_192', to_jsonb(''::text), true),
    ('pwa.icon_512', to_jsonb(''::text), true),
    ('pwa.maskable_icon', to_jsonb(''::text), true),
    ('pwa.shortcuts', to_jsonb('[]'::json), true),
    ('pwa.screenshots', to_jsonb('[]'::json), true),
    ('pwa.categories', to_jsonb('social'::text), true),
    ('pwa.offline_enabled', to_jsonb(true), true),
    ('pwa.install_prompt_enabled', to_jsonb(true), true),
    ('pwa.badge_enabled', to_jsonb(true), true),
    ('pwa.share_target_enabled', to_jsonb(false), true)
ON CONFLICT (key) DO NOTHING;

COMMIT;
