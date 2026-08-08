-- 000328_seed_appointment_alert_rbac.up.sql
-- RBAC nodes for Embassy Appointment Alerter (§91, ADR-099).
--   appointment_alert.create      — member+
--   appointment_alert.manage_own  — member+
--   appointment_alert.manage_any  — admin, super_admin
--   embassy_monitor.manage        — admin, super_admin

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('appointment_alert.create',     'Create appointment availability alerts (member+)'),
    ('appointment_alert.manage_own', 'Manage own appointment alerts (member+)'),
    ('appointment_alert.manage_any', 'Manage any appointment alerts (admin+)'),
    ('embassy_monitor.manage',       'Manage monitored embassy configs (admin+)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'super_admin')
  AND p.key IN ('appointment_alert.create','appointment_alert.manage_own')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key IN ('appointment_alert.manage_any','embassy_monitor.manage')
ON CONFLICT DO NOTHING;

COMMIT;
