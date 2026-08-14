-- 000459_remove_blog_review_create_from_member.up.sql
-- BUG-008 (blog audit): blog.review.create was granted to the member role
-- by migration 000175, but CreateReview (POST /api/v1/blog/reviews) is an
-- editorial/admin action that creates a content_reviews row. Regular
-- members should not be able to create editorial reviews on any content —
-- that pollutes the admin review-queue. The author-facing action is
-- SubmitForReview, which requires blog.create (correct) and creates an
-- unassigned review row, not blog.review.create.
--
-- Super-admin is unaffected (IsSuperAdmin bypass).

BEGIN;

DELETE FROM role_permissions
WHERE permission_id = (SELECT id FROM permissions WHERE key = 'blog.review.create')
  AND role_id IN (SELECT id FROM user_roles WHERE slug IN ('member', 'member_role'));

COMMIT;
