-- Add missing Control App navigation keys
INSERT INTO translations (tenant_id, locale, "key", value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'en', 'nav.dlq', 'Dead Letter Queue'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'nav.feature_flags', 'Feature Flags'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'nav.tenants', 'Tenants'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'nav.ai_budget', 'AI Budget'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'nav.cron', 'Cron Jobs')
ON CONFLICT (tenant_id, locale, "key") DO UPDATE SET value = EXCLUDED.value, updated_at = now();
