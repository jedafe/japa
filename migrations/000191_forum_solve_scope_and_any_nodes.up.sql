-- 000191_forum_solve_scope_and_any_nodes.up.sql
-- BUG-016: forum.thread.solve was over-granted to members (via 000175's
-- member_role grant, which used slug='member_role'). Remove it from any
-- member-level role so the runtime OP/moderator check in the service is
-- the sole enforcement. The route gate changes from RequirePermission to
-- RequireAuth so the OP (a regular member) can still attempt the action;
-- the service enforces author/space-moderator/permission-holder scope.
--
-- BUG-019: Reintroduce the forum _any permission nodes that 000176 removed
-- as "dead duplicates". They are NOT dead — the forum service uses them as
-- moderator-override nodes for edit/delete (canModerateThread/canModerateReply).
-- Without them, only the author and space moderators can edit/delete; global
-- admins without the wildcard cannot. Super-admin already passes via the "*"
-- wildcard; these nodes exist so custom admin roles can be granted explicit
-- forum moderation capabilities.

BEGIN;

-- BUG-016: remove forum.thread.solve from member-level roles.
DELETE FROM role_permissions
WHERE permission_id = (SELECT id FROM permissions WHERE key = 'forum.thread.solve')
  AND role_id IN (SELECT id FROM user_roles WHERE slug IN ('member', 'member_role'));

-- BUG-019: re-seed the forum _any nodes.
INSERT INTO permissions (key, description) VALUES
    ('forum.thread.edit_any', 'Edit any forum thread (moderator override)'),
    ('forum.thread.delete_any', 'Delete any forum thread (moderator override)'),
    ('forum.reply.edit_any', 'Edit any forum reply (moderator override)'),
    ('forum.reply.delete_any', 'Delete any forum reply (moderator override)')
ON CONFLICT (key) DO NOTHING;

COMMIT;
