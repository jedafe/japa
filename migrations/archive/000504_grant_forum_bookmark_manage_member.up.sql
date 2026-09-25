-- 000504_grant_forum_bookmark_manage_member.up.sql
-- Fix BUG-003: `forum.bookmark.manage` was granted in 000175 to
-- `WHERE r.slug = 'member_role'`, but the actual member role slug is
-- `member` (see 000099 seed_data.up.sql line 398: ('...0001', 'Member',
-- 'member')). The 000175 grant therefore matched zero rows and no regular
-- member ever received the node. As a result POST /forum/bookmarks and
-- DELETE /forum/bookmarks/{id} return 403 for every non-super-admin
-- member. 000191 cleaned up the same slug drift for forum.thread.solve
-- but did not re-grant forum.bookmark.manage.
--
-- This migration grants forum.bookmark.manage to member+ roles so the
-- self-service forum bookmark routes work for members while remaining
-- revocable per §03/§26. Mirrors the 000458 member+ grant pattern.

BEGIN;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'professional', 'agent', 'moderator', 'admin')
  AND p.key = 'forum.bookmark.manage'
ON CONFLICT DO NOTHING;

COMMIT;
