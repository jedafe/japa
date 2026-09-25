-- Reverse: remove the appearance/branding settings seeded by the up migration.
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN (
    'appearance.primary_color',
    'appearance.logo_url',
    'appearance.favicon_url',
    'appearance.dark_mode_default',
    'appearance.feed_card_style',
    'general.site_name',
    'general.site_description',
    'general.maintenance_mode',
    'general.registration_open',
    'onboarding.min_tags_follow',
    'onboarding.min_users_follow',
    'onboarding.min_spaces_join',
    'i18n.default_language',
    'i18n.enabled_languages'
  );
