-- 001020_drop_content_edit_permission.down.sql
-- Restores the legacy `content.edit` node and its original `member` role
-- grant (as seeded in 000099).

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('content.edit', 'Edit content')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug = 'member'
  AND p.key = 'content.edit'
ON CONFLICT DO NOTHING;

COMMIT;
