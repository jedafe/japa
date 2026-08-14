-- 000458_grant_bookmarks_manage_member_plus.up.sql
-- Fix BUG-BF-02: The `bookmarks.manage` permission ("Manage bookmark
-- folders") was seeded in 000099 and re-confirmed in 000109, but the 000109
-- reconciliation dropped it from the member role grant list (only
-- `bookmarks.create` was kept). As a result no role held the node and the
-- /me/bookmark-folders POST/PATCH/DELETE routes enforced no permission beyond
-- auth.RequireAuth. The routes are now wired to RequirePermission("bookmarks.manage")
-- (server.go); this migration grants the node to all member+ roles so the
-- standard self-service folder CRUD continues to work for members while
-- remaining revocable per §03/§26. Mirrors the 000456 member+ grant pattern.

BEGIN;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'professional', 'agent', 'moderator', 'admin')
  AND p.key = 'bookmarks.manage'
ON CONFLICT DO NOTHING;

COMMIT;
