-- 000832_grant_member_academy_self_serve_perms.up.sql
-- Grants the academy.* self-service permission nodes to the member role.
-- Archive migration 000175 intended these grants for university.* nodes but
-- targeted role slug 'member_role' (actual slug: 'member'), so the INSERT
-- silently matched zero rows. After the 000800 university→academy rename the
-- member role still only holds 'academy.enroll', which blocks the entire
-- instructor-application and course-creation flow for regular members.
--
-- Grants per .jeos/conventions/permission-key-naming.md (Japa Academy):
--   academy.instructor.apply   → member+  (apply to become a creator)
--   academy.course.create      → member+  (approved-instructor check in handler)
--   academy.course.submit      → member+  (approved-instructor check in handler)
--   academy.review.create      → member+  (enrollment check in handler)
--   academy.payout.view_own    → member+  (instructor-profile check in handler)
--
-- super_admin is unaffected (covered by the '*' wildcard).

BEGIN;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
JOIN permissions p ON p.key IN (
    'academy.instructor.apply',
    'academy.course.create',
    'academy.course.submit',
    'academy.review.create',
    'academy.payout.view_own'
)
WHERE r.slug = 'member'
ON CONFLICT DO NOTHING;

COMMIT;
