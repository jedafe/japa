-- 000144_seed_rbac_gate_permissions.down.sql
-- Removes the permission nodes seeded in the up migration.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN (
        'comments.create', 'comments.edit_own', 'comments.edit_any',
        'comments.delete_own', 'comments.delete_any', 'comments.vote',
        'organizations.edit_own', 'organizations.delete_own',
        'organizations.members.manage', 'organizations.invite',
        'organizations.claim', 'forum.tag.assign', 'dm.delete',
        'content.share', 'interactions.block.manage', 'interactions.mute.manage'
    )
);

DELETE FROM permissions WHERE key IN (
    'comments.create', 'comments.edit_own', 'comments.edit_any',
    'comments.delete_own', 'comments.delete_any', 'comments.vote',
    'organizations.edit_own', 'organizations.delete_own',
    'organizations.members.manage', 'organizations.invite',
    'organizations.claim', 'forum.tag.assign', 'dm.delete',
    'content.share', 'interactions.block.manage', 'interactions.mute.manage'
);

COMMIT;
