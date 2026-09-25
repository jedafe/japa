-- 000507_remove_orphaned_blog_rbac_nodes.down.sql
-- Reverts 000507: re-seeds the four orphaned blog.* RBAC nodes that were
-- removed. NOTE: re-seeding the nodes does NOT restore role_permissions
-- grants (those were deleted by the up migration and cannot be reconstructed
-- without knowing which roles held them). The nodes will exist but be
-- ungranted — matching their pre-removal state for most roles (they were
-- never enforced, so grants were cosmetic).

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('blog.edit_any', 'Edit any blog post (admin/editor, Section 24 granular node)'),
    ('blog.publish', 'Publish blog posts'),
    ('blog.review', 'Review blog submissions'),
    ('blog.rss.view', 'View RSS feed')
ON CONFLICT (key) DO NOTHING;

COMMIT;
