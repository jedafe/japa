-- 000695_reseed_blog_edit_permission_nodes.up.sql
-- Renumbered from 000458 — it collided with 000458_grant_bookmarks_manage_member_plus
-- and 000458_seed_calendar_categories_setting. Two changes relative to the
-- original body, both required by later migrations:
--   1. The tenant filter is gone (role/user_roles tenant_id dropped by 000668).
--   2. blog.edit_any is NOT re-seeded — 000507_remove_orphaned_blog_rbac_nodes
--      deliberately deleted it; re-adding it here would resurrect an orphaned
--      node. Only blog.edit_own (not touched by 000507) is seeded.
-- Tail-position body; databases that applied the 000458 file skip this via the
-- 000000 registry marker.

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('blog.edit_own', 'Edit own blog posts (Section 24 granular node)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
JOIN permissions p ON p.key = 'blog.edit_own'
WHERE r.slug IN ('member', 'member_role')
ON CONFLICT DO NOTHING;

COMMIT;
