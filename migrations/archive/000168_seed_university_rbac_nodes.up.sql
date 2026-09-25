-- 000168_seed_university_rbac_nodes.up.sql
-- Seeds the 12 RBAC permission nodes for the Japa University creator economy
-- (ADR-063) and grants them to the super_admin_role so the RBAC CRUD gate
-- does not fire. Companion to migration 000166.
-- Ref: japaconnect-engineering/conventions/permission-key-naming.md (Japa University)

BEGIN;

-- 1. Insert the 12 university.* permission nodes.
INSERT INTO permissions (key, description) VALUES
    ('university.instructor.apply',   'Submit an instructor (creator) application to Japa University'),
    ('university.instructor.approve', 'Approve or reject an instructor application'),
    ('university.instructor.manage',  'Manage instructors (suspend, override tier/commission, view all)'),
    ('university.course.create',      'Create a marketplace track (approved instructors only)'),
    ('university.course.submit',      'Submit a track for marketplace approval'),
    ('university.course.approve',     'Approve a submitted track to the marketplace'),
    ('university.course.feature',     'Feature/unfeature a marketplace track'),
    ('university.enrollment.view_own','View own course enrollments and progress'),
    ('university.review.create',      'Create a course review (enrolled users only)'),
    ('university.review.moderate',    'Moderate course reviews (flag/remove/restore)'),
    ('university.payout.view_own',    'View own payout history and open revenue splits'),
    ('university.payout.process',     'Trigger, retry, or mark instructor payouts paid/failed')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant the admin-scope nodes to super_admin_role (idempotent).
-- Self-serve nodes (apply, create, submit, view_own, review.create, payout.view_own)
-- are enforced at the handler via rbac.RequirePermission on the caller's own
-- resource; they are NOT granted to super_admin_role here because super_admin
-- already bypasses RBAC via the wildcard '*' node (Section 03).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin_role'
  AND p.key IN (
        'university.instructor.approve',
        'university.instructor.manage',
        'university.course.approve',
        'university.course.feature',
        'university.review.moderate',
        'university.payout.process'
  )
ON CONFLICT DO NOTHING;

COMMIT;
