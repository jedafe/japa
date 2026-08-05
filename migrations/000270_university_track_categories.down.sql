-- 000270_university_track_categories.down.sql

BEGIN;

-- Revoke the permission from super_admin_role.
DELETE FROM role_permissions
WHERE permission_id = (
    SELECT id FROM permissions WHERE key = 'university.category.manage'
)
AND role_id = (
    SELECT id FROM user_roles
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND slug = 'super_admin_role'
);

-- Remove the permission node.
DELETE FROM permissions WHERE key = 'university.category.manage';

-- Drop the table.
DROP TABLE IF EXISTS university_track_categories;

COMMIT;
