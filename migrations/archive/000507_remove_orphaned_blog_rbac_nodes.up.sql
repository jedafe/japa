-- 000507_remove_orphaned_blog_rbac_nodes.up.sql
-- BUG-003/BUG-004/BUG-005/BUG-006 (blog audit): the following blog.* RBAC
-- nodes were seeded in 000099 but are NEVER enforced anywhere in the
-- codebase. They are superseded by other nodes:
--   blog.edit_any  → admin "edit any" is covered by blog.review.manage on
--                    PATCH /admin/blog/{id} (server.go:4948).
--   blog.publish   → publish happens via POST /content/{id}/publish
--                    (content.publish) or admin approve (blog.review.manage).
--   blog.review    → superseded by blog.review.create / blog.review.manage.
--   blog.rss.view  → GET /blog/feed.xml is public (no auth) per §24.
--
-- Leaving orphaned nodes in the permissions table is misleading: auditors
-- expect every seeded node to be enforced, and role-grant UIs surface them
-- as grantable checkboxes that do nothing. This migration removes them and
-- any role_permissions grants referencing them.
--
-- Super-admin is unaffected (IsSuperAdmin bypass). No route behavior changes
-- because no route enforces these nodes.

BEGIN;

-- Remove any role_permissions grants for the orphaned nodes first (FK safety).
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN ('blog.edit_any', 'blog.publish', 'blog.review', 'blog.rss.view')
);

-- Remove the orphaned permission nodes themselves.
DELETE FROM permissions
WHERE key IN ('blog.edit_any', 'blog.publish', 'blog.review', 'blog.rss.view');

COMMIT;
