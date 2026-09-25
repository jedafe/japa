-- 000229_directory_view_rbac_nodes.down.sql
-- Reverses 000229: removes the 4 directory view permission nodes and their
-- role grants. The routes revert to plain OptionalAuth (no permission gate).

BEGIN;

-- 1. Remove role grants for the 4 directory view nodes.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN (
        'directory.users.view',
        'directory.spaces.view',
        'directory.tags.view',
        'directory.organizations.view'
    )
);

-- 2. Remove the permission nodes themselves.
DELETE FROM permissions
WHERE key IN (
    'directory.users.view',
    'directory.spaces.view',
    'directory.tags.view',
    'directory.organizations.view'
);

COMMIT;
