-- 000328_seed_appointment_alert_rbac.down.sql
BEGIN;
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE key IN (
    'appointment_alert.create','appointment_alert.manage_own',
    'appointment_alert.manage_any','embassy_monitor.manage'
));
DELETE FROM permissions WHERE key IN (
    'appointment_alert.create','appointment_alert.manage_own',
    'appointment_alert.manage_any','embassy_monitor.manage'
);
COMMIT;
