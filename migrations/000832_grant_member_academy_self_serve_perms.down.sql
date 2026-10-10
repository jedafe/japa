-- 000832_grant_member_academy_self_serve_perms.down.sql
-- Revokes the member-role academy.* self-service grants added by 000832.

BEGIN;

DELETE FROM role_permissions rp
USING user_roles r, permissions p
WHERE rp.role_id = r.id
  AND rp.permission_id = p.id
  AND r.slug = 'member'
  AND p.key IN (
      'academy.instructor.apply',
      'academy.course.create',
      'academy.course.submit',
      'academy.review.create',
      'academy.payout.view_own'
  );

COMMIT;
