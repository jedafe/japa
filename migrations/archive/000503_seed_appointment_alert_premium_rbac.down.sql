-- 000503_seed_appointment_alert_premium_rbac.down.sql
-- Remove the appointment_alert.premium RBAC node (§91, ADR-099).

BEGIN;

DELETE FROM role_permissions
WHERE permission_id = (SELECT id FROM permissions WHERE key = 'appointment_alert.premium');

DELETE FROM permissions WHERE key = 'appointment_alert.premium';

COMMIT;
