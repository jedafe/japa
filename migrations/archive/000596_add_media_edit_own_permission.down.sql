-- 000596_add_media_edit_own_permission.down.sql
-- Reverses 000596: removes media.edit_own role grants and the permission node.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id = (SELECT id FROM permissions WHERE key = 'media.edit_own');

DELETE FROM permissions WHERE key = 'media.edit_own';

COMMIT;
