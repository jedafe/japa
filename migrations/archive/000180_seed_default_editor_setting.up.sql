-- Seed the appearance.default_editor system setting (is_public = true) so the
-- main site composer can read the admin-configured default editor mode from
-- GET /site/settings/public. Ref: japaconnect-engineering/conventions/system-settings-catalog.md (Appearance)
-- Default tenant UUID: '00000000-0000-0000-0000-000000000001'

INSERT INTO system_settings (tenant_id, key, value, is_public) VALUES
    ('00000000-0000-0000-0000-000000000001', 'appearance.default_editor', '"block"', true)
ON CONFLICT (tenant_id, key) DO UPDATE SET is_public = true;
