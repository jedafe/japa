-- 000319_seed_ama_rbac.down.sql
BEGIN;
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN ('forum.ama.create','forum.ama.moderate'));
DELETE FROM permissions WHERE key IN ('forum.ama.create','forum.ama.moderate');
COMMIT;
