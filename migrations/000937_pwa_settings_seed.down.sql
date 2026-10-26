-- 000937_pwa_settings_seed.down.sql
-- Remove the seeded §124 PWA settings rows.

BEGIN;

DELETE FROM system_settings WHERE key IN (
    'pwa.name',
    'pwa.short_name',
    'pwa.description',
    'pwa.theme_color',
    'pwa.background_color',
    'pwa.display',
    'pwa.start_url',
    'pwa.icon_192',
    'pwa.icon_512',
    'pwa.maskable_icon',
    'pwa.shortcuts',
    'pwa.screenshots',
    'pwa.categories',
    'pwa.offline_enabled',
    'pwa.install_prompt_enabled',
    'pwa.badge_enabled',
    'pwa.share_target_enabled'
);

COMMIT;
