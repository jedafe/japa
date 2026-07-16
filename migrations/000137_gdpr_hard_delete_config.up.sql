-- 000137_gdpr_hard_delete_config.up.sql
-- Seeds GDPR hard-delete configuration settings per ADR-054.
-- gdpr.hard_delete_enabled controls whether physical deletion follows anonymization.
-- gdpr.retention_days sets optional delay before hard delete (0 = immediate).

BEGIN;

INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'gdpr.hard_delete_enabled', '"false"'),
    ('00000000-0000-0000-0000-000000000001', 'gdpr.retention_days', '"0"')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
