-- Seed appearance/branding system settings with is_public = true so the
-- main site can render the primary color and logo from control panel edits.
-- Ref: japaconnect-engineering/conventions/system-settings-catalog.md (Appearance)
-- Default tenant UUID: '00000000-0000-0000-0000-000000000001'

INSERT INTO system_settings (tenant_id, key, value, is_public) VALUES
    ('00000000-0000-0000-0000-000000000001', 'appearance.primary_color',     '"#c615a6"', true),
    ('00000000-0000-0000-0000-000000000001', 'appearance.logo_url',          '""',        true),
    ('00000000-0000-0000-0000-000000000001', 'appearance.favicon_url',       '""',        true),
    ('00000000-0000-0000-0000-000000000001', 'appearance.dark_mode_default', 'false',     true),
    ('00000000-0000-0000-0000-000000000001', 'appearance.feed_card_style',   '"reddit"',  true),
    ('00000000-0000-0000-0000-000000000001', 'general.site_name',            '"JapaConnect"', true),
    ('00000000-0000-0000-0000-000000000001', 'general.site_description',     '""',        true),
    ('00000000-0000-0000-0000-000000000001', 'general.maintenance_mode',     'false',     true),
    ('00000000-0000-0000-0000-000000000001', 'general.registration_open',    'true',      true),
    ('00000000-0000-0000-0000-000000000001', 'onboarding.min_tags_follow',   '"3"',       true),
    ('00000000-0000-0000-0000-000000000001', 'onboarding.min_users_follow',  '"2"',       true),
    ('00000000-0000-0000-0000-000000000001', 'onboarding.min_spaces_join',   '"2"',       true),
    ('00000000-0000-0000-0000-000000000001', 'i18n.default_language',        '"en"',      true),
    ('00000000-0000-0000-0000-000000000001', 'i18n.enabled_languages',       '"en,fr"',   true)
ON CONFLICT (tenant_id, key) DO UPDATE SET is_public = true;
