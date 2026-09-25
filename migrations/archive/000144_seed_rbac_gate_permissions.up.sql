-- 000144_seed_rbac_gate_permissions.up.sql
-- Seeds permission nodes enforced by RBAC gates added in the same commit.
-- Covers: comments, organizations, DMs, interactions, content share, forum tags.
-- Grants self-service permissions to the members group; admin-only to super_admin.

BEGIN;

-- 1. Insert new permission nodes.
INSERT INTO permissions (key, description) VALUES
    ('comments.create', 'Create comments on content'),
    ('comments.edit_own', 'Edit own comments'),
    ('comments.edit_any', 'Edit any comment (admin/moderator)'),
    ('comments.delete_own', 'Delete own comments'),
    ('comments.delete_any', 'Delete any comment (admin/moderator)'),
    ('comments.vote', 'Upvote or downvote comments'),
    ('organizations.edit_own', 'Edit own organizations'),
    ('organizations.delete_own', 'Delete own organizations'),
    ('organizations.members.manage', 'Add, remove, and update organization members'),
    ('organizations.invite', 'Send and manage organization invitations'),
    ('organizations.claim', 'Claim ownership of an organization'),
    ('forum.tag.assign', 'Assign or remove tags on forum threads'),
    ('dm.delete', 'Delete own direct messages'),
    ('content.share', 'Share content'),
    ('interactions.block.manage', 'Block and unblock users'),
    ('interactions.mute.manage', 'Mute and unmute users')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant self-service permissions to the members group.
INSERT INTO group_roles (group_id, role_id)
SELECT g.id, r.id
FROM user_groups g
JOIN user_roles r ON r.tenant_id = g.tenant_id AND r.slug = 'member_role'
WHERE g.slug = 'members'
ON CONFLICT DO NOTHING;

-- Create a temporary role for the new member permissions if it doesn't exist,
-- then map the permissions to the member_role via role_permissions.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
JOIN permissions p ON p.key IN (
    'comments.create',
    'comments.edit_own',
    'comments.delete_own',
    'comments.vote',
    'organizations.edit_own',
    'organizations.delete_own',
    'organizations.members.manage',
    'organizations.invite',
    'organizations.claim',
    'forum.tag.assign',
    'dm.delete',
    'content.share',
    'interactions.block.manage',
    'interactions.mute.manage'
)
WHERE r.slug = 'member_role'
ON CONFLICT DO NOTHING;

-- 3. Grant all new permissions to the super_admin role.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
JOIN permissions p ON p.key IN (
    'comments.create',
    'comments.edit_own',
    'comments.edit_any',
    'comments.delete_own',
    'comments.delete_any',
    'comments.vote',
    'organizations.edit_own',
    'organizations.delete_own',
    'organizations.members.manage',
    'organizations.invite',
    'organizations.claim',
    'forum.tag.assign',
    'dm.delete',
    'content.share',
    'interactions.block.manage',
    'interactions.mute.manage'
)
WHERE r.slug = 'super_admin_role'
ON CONFLICT DO NOTHING;

COMMIT;
