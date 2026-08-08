-- 000311_seed_digest_rbac.down.sql
BEGIN;
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE key = 'digest.manage');
DELETE FROM permissions WHERE key = 'digest.manage';
COMMIT;
