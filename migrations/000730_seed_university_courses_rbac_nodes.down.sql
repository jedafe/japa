-- 000730_seed_university_courses_rbac_nodes.down.sql
-- Reverses 000730: removes universities.course_catalog.* / course_offering.*
-- grants and permission nodes.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN (
        'universities.course_catalog.list',
        'universities.course_catalog.view',
        'universities.course_catalog.create',
        'universities.course_catalog.update',
        'universities.course_catalog.delete',
        'universities.course_offering.list',
        'universities.course_offering.view',
        'universities.course_offering.create',
        'universities.course_offering.update',
        'universities.course_offering.delete'
    )
);

DELETE FROM permissions
WHERE key IN (
    'universities.course_catalog.list',
    'universities.course_catalog.view',
    'universities.course_catalog.create',
    'universities.course_catalog.update',
    'universities.course_catalog.delete',
    'universities.course_offering.list',
    'universities.course_offering.view',
    'universities.course_offering.create',
    'universities.course_offering.update',
    'universities.course_offering.delete'
);

COMMIT;
