-- 000175_grant_member_role_missing_permissions.down.sql
-- Reverts 000175: removes the self-service permissions granted to member_role.

BEGIN;

DELETE FROM role_permissions
WHERE role_id IN (SELECT id FROM user_roles WHERE slug = 'member_role')
  AND permission_id IN (
    SELECT id FROM permissions WHERE key IN (
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
  );

COMMIT;
