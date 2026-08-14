-- 000458_reseed_blog_edit_permission_nodes.up.sql
-- BUG-001 (blog audit): migration 000176 removed blog.edit_own and
-- blog.edit_any as "dead duplicates" of content.edit, but the routes
-- PATCH /api/v1/blog/{id} and DELETE /api/v1/blog/{id} still enforce
-- blog.edit_own (internal/server/server.go:2848,2850). Section 24
-- (Blog Extension — Locked) explicitly mandates granular permission
-- nodes "blog.create, blog.edit_own, blog.edit_any, blog.publish,
-- blog.feature through dynamic RBAC rather than fixed roles". Removing
-- them broke own-post edit/delete for every non-super-admin author.
--
-- This mirrors the 000191 fix that re-seeded the forum _any nodes 000176
-- also wrongly removed. Super-admin is unaffected (IsSuperAdmin bypass).

BEGIN;

-- Re-seed the two §24-mandated blog edit permission nodes.
INSERT INTO permissions (key, description) VALUES
    ('blog.edit_own', 'Edit own blog posts (Section 24 granular node)'),
    ('blog.edit_any', 'Edit any blog post (admin/editor, Section 24 granular node)')
ON CONFLICT (key) DO NOTHING;

-- Grant blog.edit_own to the member-level role so regular authors can
-- edit/delete their own blog posts. Use both slug variants for safety
-- (see 000191 which handles the member vs member_role naming drift).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
JOIN permissions p ON p.key = 'blog.edit_own'
WHERE r.slug IN ('member', 'member_role')
ON CONFLICT DO NOTHING;

COMMIT;
