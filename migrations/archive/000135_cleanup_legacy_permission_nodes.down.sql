-- 000135_cleanup_legacy_permission_nodes.down.sql
-- Reverts 000135: removes the feature.* member grants and restores
-- dot-scoped legacy permission nodes. 

BEGIN;

-- 1. Remove feature.* permission grants for the member role.
DELETE FROM role_permissions
WHERE role_id IN (
    SELECT id FROM user_roles
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND slug = 'member'
)
AND permission_id IN (
    SELECT id FROM permissions WHERE key LIKE 'feature.%'
);

-- 2. Restore dot-scoped legacy permission nodes.
INSERT INTO permissions (key, description) VALUES
    ('blog.edit.any', 'Blog Edit Any (legacy dot-scoped)'),
    ('blog.edit.own', 'Blog Edit Own (legacy dot-scoped)'),
    ('content.delete.any', 'Content Delete Any (legacy dot-scoped)'),
    ('content.delete.own', 'Content Delete Own (legacy dot-scoped)'),
    ('content.edit.any', 'Content Edit Any (legacy dot-scoped)'),
    ('content.edit.own', 'Content Edit Own (legacy dot-scoped)'),
    ('media.delete.any', 'Media Delete Any (legacy dot-scoped)'),
    ('media.delete.own', 'Media Delete Own (legacy dot-scoped)')
ON CONFLICT (key) DO NOTHING;

COMMIT;
