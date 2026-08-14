-- 000459_remove_blog_review_create_from_member.down.sql
-- Reverts 000459: re-grants blog.review.create to the member role.
-- WARNING: reverting this re-introduces BUG-008 (members can create
-- editorial reviews on any content).

BEGIN;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
JOIN permissions p ON p.key = 'blog.review.create'
WHERE r.slug IN ('member', 'member_role')
ON CONFLICT DO NOTHING;

COMMIT;
