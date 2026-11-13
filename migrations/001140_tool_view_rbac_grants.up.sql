-- 001140_tool_view_rbac_grants.up.sql
-- Grants the tool .view nodes to the member-tier roles they were always
-- documented for. credential_checker.view was seeded by 000332 ("all users")
-- but never granted to any role; eca_guide.view was seeded+granted by 000713
-- to member/super_admin only. With the routes now mounted under
-- rbac.OptionalPermission (public stays open for guests, authed users need
-- the node — same convention as directory.spaces.view), ungranted member
-- roles would 403 on every tool read. Mirrors 001018 (follows.view) and
-- 000229 (directory view nodes): grant broadly so admins can revoke per
-- group to hide a tool from that group.

BEGIN;

-- credential_checker.view — no prior grants; cover every member-tier role
-- plus guest so "all users" (the seeded intent) holds for authed users.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('member', 'pro_member', 'business_member', 'guest', 'super_admin')
  AND p.key = 'credential_checker.view'
ON CONFLICT DO NOTHING;

-- eca_guide.view — top up the tier/guest roles missing from 000713.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('pro_member', 'business_member', 'guest')
  AND p.key = 'eca_guide.view'
ON CONFLICT DO NOTHING;

COMMIT;
