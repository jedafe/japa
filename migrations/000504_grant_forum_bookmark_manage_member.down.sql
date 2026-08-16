-- 000504_grant_forum_bookmark_manage_member.down.sql
-- Reverses 000504: removes forum.bookmark.manage from member+ roles.
-- Note: this restores the buggy state where members lack the node; only
-- run during a deliberate rollback.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id = (SELECT id FROM permissions WHERE key = 'forum.bookmark.manage')
  AND role_id IN (
      SELECT id FROM user_roles
      WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
        AND slug IN ('member', 'professional', 'agent', 'moderator', 'admin')
  );

COMMIT;
