-- 000695_reseed_blog_edit_permission_nodes.down.sql
-- Renumbered from 000458 (duplicate version prefix). See the .up file.
-- 000458_reseed_blog_edit_permission_nodes.down.sql
-- Reverts 000458: removes the re-seeded blog.edit_own / blog.edit_any
-- nodes and their member-role grants. WARNING: reverting this will
-- re-introduce BUG-001 (non-super-admin authors cannot edit/delete
-- their own blog posts).

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN ('blog.edit_own', 'blog.edit_any')
)
  AND role_id IN (SELECT id FROM user_roles WHERE slug IN ('member', 'member_role'));

DELETE FROM permissions WHERE key IN ('blog.edit_own', 'blog.edit_any');

COMMIT;
