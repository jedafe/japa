-- 000140_seed_media_edit_permission.up.sql
-- Seeds the media.edit_any RBAC permission node and grants it to super_admin_role.
-- Fixes Critical #1: PATCH /admin/media/{id} was gated on an unseeded permission,
-- causing 403 for everyone including super_admin.

BEGIN;

-- 1. Insert the permission node.
INSERT INTO permissions (key, description) VALUES
    ('media.edit_any', 'Edit any media (admin)')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant to super_admin_role.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin_role'
  AND p.key = 'media.edit_any'
ON CONFLICT DO NOTHING;

COMMIT;
