-- 000622_content_feature_permission.up.sql
-- Seed the generic content featuring node.
-- Ref: sections/03-rbac.md (Generic content featuring), sections/14-content-type-system.md
--      (Generic featured list + featuring).
--
-- `content.feature` gates POST/DELETE /content/{id}/feature — the generic
-- counterpart of `blog.feature` for the shared `content_featured` junction.
-- Default grant: super_admin only (moderators opt in via role grants).

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('content.feature', 'Content Feature')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key = 'content.feature'
ON CONFLICT DO NOTHING;

COMMIT;
