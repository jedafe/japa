-- 000305_seed_scam_alert_rbac.down.sql
BEGIN;
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN ('scam_alert.create','scam_alert.review','scam_alert.pin'));
DELETE FROM permissions WHERE key IN ('scam_alert.create','scam_alert.review','scam_alert.pin');
COMMIT;
