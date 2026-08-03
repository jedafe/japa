-- 000250_remittance_tracker.down.sql
-- Reverses 000250_remittance_tracker.up.sql.

BEGIN;

-- Revoke remittance.* grants.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin', 'member')
  AND rp.permission_id = p.id
  AND p.key IN ('remittance.view', 'remittance.alert', 'remittance.manage', 'remittance.fetch');

-- Remove remittance.* permission nodes.
DELETE FROM permissions WHERE key IN ('remittance.view', 'remittance.alert', 'remittance.manage', 'remittance.fetch');

-- Remove remittance.* system_settings.
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('remittance.enabled', 'remittance.fetch_interval_minutes', 'remittance.manual_stale_hours',
              'remittance.history_retention_days', 'remittance.default_send_currency');

-- Drop tables in reverse dependency order.
DROP TABLE IF EXISTS remittance_rate_alerts;
DROP TABLE IF EXISTS remittance_rates;
DROP TABLE IF EXISTS remittance_providers;

COMMIT;
