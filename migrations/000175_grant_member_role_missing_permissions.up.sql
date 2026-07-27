-- 000175_grant_member_role_missing_permissions.up.sql
-- AUDIT-FRESH-BE-002: member_role was missing self-service permissions that
-- regular members need to use platform features. These are all user-facing
-- routes (not /admin/) where the handler checks ownership internally after
-- the RBAC gate passes. Super-admin is unaffected (covered by "*" wildcard).
--
-- Permissions granted:
--   content: delete own, schedule own
--   forum: solve own threads, create polls, create/edit tags, bookmark threads
--   blog: create/edit/delete series, submit for review
--   reviews: flag inappropriate reviews
--   spaces: edit/delete own spaces
--   payments: manage own subscriptions
--   professional_profile: manage members, invite, switch author
--   comparison: manage items/dimensions/values
--   university: apply as instructor, submit courses, review courses, view payouts

BEGIN;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
JOIN permissions p ON p.key IN (
    'content.delete',
    'content.schedule',
    'forum.thread.solve',
    'forum.poll.create',
    'forum.tag.create',
    'forum.tag.edit',
    'forum.bookmark.manage',
    'blog.series.create',
    'blog.series.edit',
    'blog.series.delete',
    'blog.review.create',
    'reviews.flag',
    'spaces.edit',
    'spaces.delete',
    'payments.subscriptions.manage',
    'professional_profile.member.manage',
    'professional_profile.invite',
    'professional_profile.author.switch',
    'comparison.item.manage',
    'comparison.dimension.manage',
    'comparison.value.manage',
    'university.instructor.apply',
    'university.course.submit',
    'university.review.create',
    'university.payout.view_own'
)
WHERE r.slug = 'member_role'
ON CONFLICT DO NOTHING;

COMMIT;
