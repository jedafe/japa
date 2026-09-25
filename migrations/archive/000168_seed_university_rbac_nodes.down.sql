-- Rollback: remove the 12 university.* RBAC nodes seeded in 000168.
-- role_permissions rows cascade-delete via FK if the permissions table row is
-- removed, but we delete them explicitly first to be safe across engines.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN (
        'university.instructor.apply',
        'university.instructor.approve',
        'university.instructor.manage',
        'university.course.create',
        'university.course.submit',
        'university.course.approve',
        'university.course.feature',
        'university.enrollment.view_own',
        'university.review.create',
        'university.review.moderate',
        'university.payout.view_own',
        'university.payout.process'
    )
);

DELETE FROM permissions
WHERE key IN (
    'university.instructor.apply',
    'university.instructor.approve',
    'university.instructor.manage',
    'university.course.create',
    'university.course.submit',
    'university.course.approve',
    'university.course.feature',
    'university.enrollment.view_own',
    'university.review.create',
    'university.review.moderate',
    'university.payout.view_own',
    'university.payout.process'
);

COMMIT;
