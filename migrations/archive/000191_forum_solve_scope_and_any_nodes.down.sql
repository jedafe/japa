-- 000191_forum_solve_scope_and_any_nodes.down.sql
-- Reverts 000191: re-grants forum.thread.solve to member roles and removes
-- the _any nodes. Note: re-granting to member roles restores the over-grant
-- bug (BUG-016); this down migration exists for completeness only.

BEGIN;

-- Re-grant forum.thread.solve to member-level roles.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
JOIN permissions p ON p.key = 'forum.thread.solve'
WHERE r.slug IN ('member', 'member_role')
ON CONFLICT DO NOTHING;

-- Remove the _any nodes (and their grants).
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN (
        'forum.thread.edit_any',
        'forum.thread.delete_any',
        'forum.reply.edit_any',
        'forum.reply.delete_any'
    )
);

DELETE FROM permissions WHERE key IN (
    'forum.thread.edit_any',
    'forum.thread.delete_any',
    'forum.reply.edit_any',
    'forum.reply.delete_any'
);

COMMIT;
