-- Seed missing public system settings used by the main app.
-- Ref: japaconnect-engineering/conventions/system-settings-catalog.md
-- Default tenant UUID: '00000000-0000-0000-0000-000000000001'

INSERT INTO system_settings (tenant_id, key, value, is_public) VALUES
    ('00000000-0000-0000-0000-000000000001', 'site.custom_css',           '""',        true),
    ('00000000-0000-0000-0000-000000000001', 'site.custom_js',            '""',        true),
    ('00000000-0000-0000-0000-000000000001', 'content.pagination.style',  '"infinite_scroll"', true)
ON CONFLICT (tenant_id, key) DO UPDATE SET is_public = true;
