-- 000207_seed_user_self_service_rbac_nodes.up.sql
-- Seeds the 4 self-service RBAC nodes from Section 50 and grants them to
-- both super_admin (holds all) and member (default: all members).
-- Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md lines 262-265

BEGIN;

-- 1. Ensure the permission nodes exist.
INSERT INTO permissions (key, description) VALUES
    ('user.profile.edit_own', 'Edit own profile (display name, avatar, bio)'),
    ('user.account.manage_own', 'Manage own account (username, email, password)'),
    ('user.data.export_own', 'Request own GDPR data export'),
    ('user.account.delete_own', 'Delete own account (GDPR erasure request)')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant to super_admin_role (super_admin holds all nodes).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key IN (
    'user.profile.edit_own',
    'user.account.manage_own',
    'user.data.export_own',
    'user.account.delete_own'
  )
ON CONFLICT DO NOTHING;

-- 3. Grant to member role (default: all members per Section 50).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key IN (
    'user.profile.edit_own',
    'user.account.manage_own',
    'user.data.export_own',
    'user.account.delete_own'
  )
ON CONFLICT DO NOTHING;

COMMIT;
