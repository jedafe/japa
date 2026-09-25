-- Re-seed the removed permission nodes and restore member role grants.
INSERT INTO permissions (key, description) VALUES
    ('forum.create', 'Create forum threads and replies'),
    ('organizations.manage', 'Manage organization members and invitations')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key IN ('forum.create', 'organizations.manage')
ON CONFLICT DO NOTHING;
