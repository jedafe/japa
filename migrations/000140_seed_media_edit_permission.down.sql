-- 000140_seed_media_edit_permission.down.sql
-- Removes the media.edit_any permission node.

BEGIN;

DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE key = 'media.edit_any'
);
DELETE FROM permissions WHERE key = 'media.edit_any';

COMMIT;
