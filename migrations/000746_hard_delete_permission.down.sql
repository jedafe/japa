-- 000746_hard_delete_permission.down.sql
BEGIN;

DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id AND p.key = 'admin.hard_delete';

DELETE FROM permissions WHERE key = 'admin.hard_delete';

COMMIT;
