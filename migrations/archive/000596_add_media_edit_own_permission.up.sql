-- 000596_add_media_edit_own_permission.up.sql
-- Adds the media.edit_own permission node and grants it to the default
-- member and super_admin roles. This node is required by PATCH /api/v1/media/{id}
-- which was previously gated with media.library.view_own (a view-only node).
--
-- Ref: japaconnect-engineering/sections/03-rbac.md
--      japaconnect-engineering/sections/34-media-system.md

BEGIN;

INSERT INTO permissions (key, description)
VALUES ('media.edit_own', 'Edit own uploaded media metadata (alt_text, caption)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key = 'media.edit_own'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key = 'media.edit_own'
ON CONFLICT DO NOTHING;

COMMIT;
