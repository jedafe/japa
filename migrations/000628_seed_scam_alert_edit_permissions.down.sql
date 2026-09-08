-- 000628_seed_scam_alert_edit_permissions.down.sql
-- Reverses 000628: removes the scam_alert.edit_own/edit_any role grants and
-- deletes the permission nodes.

BEGIN;

DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.key IN ('scam_alert.edit_own', 'scam_alert.edit_any');

DELETE FROM permissions
WHERE key IN ('scam_alert.edit_own', 'scam_alert.edit_any');

COMMIT;
