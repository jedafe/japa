-- 1. Grant all feature.* RBAC nodes to members so RequirePermission gates work alongside
-- features.Gate (tenant toggle = 404; user permission = 403).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key LIKE 'feature.%'
ON CONFLICT DO NOTHING;

-- 2. Remove dot-scoped legacy permission nodes that have equivalent underscore-scoped variants.
-- These were superseded (seed comment in 000099: ".own/.any granularity was superseded").
-- Each removed node has an underscore equivalent already seeded (e.g. blog.edit_own replaces blog.edit.own).
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN (
        'blog.edit.any', 'blog.edit.own',
        'content.delete.any', 'content.delete.own',
        'content.edit.any', 'content.edit.own',
        'media.delete.any', 'media.delete.own'
    )
);
DELETE FROM permissions WHERE key IN (
    'blog.edit.any', 'blog.edit.own',
    'content.delete.any', 'content.delete.own',
    'content.edit.any', 'content.edit.own',
    'media.delete.any', 'media.delete.own'
);
