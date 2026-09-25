-- 000229_directory_view_rbac_nodes.up.sql
-- Directory visibility RBAC nodes (Section 03 amendment).
-- These 4 nodes gate the public directory list pages (GET /users, GET /spaces,
-- GET /tags, GET /organizations) for AUTHENTICATED users. Guests always pass
-- through (the pages remain public for SEO/anonymous browsing). Authed users
-- whose roles have had the node revoked get 403 forbidden.
--
-- Granted to member + guest by default so existing behavior is preserved
-- (everyone can see the directories). Admins can revoke the node from a
-- specific user group's role to hide that directory from that group.
-- Super_admin bypasses all permission checks in the resolver.
--
-- Idempotent; safe to re-run.

BEGIN;

-- 1. Seed the 4 directory view permission nodes.
INSERT INTO permissions (key, description) VALUES
    ('directory.users.view', 'View the public user directory at GET /users'),
    ('directory.spaces.view', 'View the public spaces directory at GET /spaces'),
    ('directory.tags.view', 'View the public tags directory at GET /tags'),
    ('directory.organizations.view', 'View the public organizations directory at GET /organizations')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant all 4 nodes to the member and guest roles (default = visible to
--    everyone). Admins can revoke from a group's role to hide a directory.
--    Super_admin is handled by the wildcard bypass in the RBAC resolver.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'guest')
  AND p.key IN (
    'directory.users.view',
    'directory.spaces.view',
    'directory.tags.view',
    'directory.organizations.view'
  )
ON CONFLICT DO NOTHING;

COMMIT;
