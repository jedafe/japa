-- 000454_appointment_alert_rbac_backfill.down.sql
-- Reverse: remove the backfilled role_permissions for professional, agent,
-- moderator, and admin roles on appointment_alert.create and
-- appointment_alert.manage_own. This restores the pre-backfill state.
-- Note: the 000328 seed (which this backfills) is not reversed here.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN ('appointment_alert.create', 'appointment_alert.manage_own')
)
AND role_id IN (
    SELECT id FROM user_roles
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND slug IN ('professional', 'agent', 'moderator', 'admin')
);

COMMIT;
