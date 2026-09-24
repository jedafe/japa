-- 000730_seed_university_courses_rbac_nodes.up.sql
-- Seeds the 10 RBAC permission nodes for the institutional university course
-- catalog + offerings admin CRUD (Section 39 — universities.course_catalog.* /
-- universities.course_offering.*). Distinct from university.course.* which
-- belongs to the Japa University creator economy (ADR-063).
-- Ref: japaconnect-engineering/sections/39-space-types-university.md

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('universities.course_catalog.list',   'List course catalog entries in the admin panel'),
    ('universities.course_catalog.view',   'View a single course catalog entry (admin)'),
    ('universities.course_catalog.create', 'Create course catalog entries'),
    ('universities.course_catalog.update', 'Update course catalog entries'),
    ('universities.course_catalog.delete', 'Soft-delete course catalog entries'),
    ('universities.course_offering.list',   'List course offerings in the admin panel'),
    ('universities.course_offering.view',   'View a single course offering (admin)'),
    ('universities.course_offering.create', 'Create course offerings'),
    ('universities.course_offering.update', 'Update course offerings (incl. bulk publish/unpublish)'),
    ('universities.course_offering.delete', 'Soft-delete course offerings')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('admin', 'super_admin')
  AND p.key IN (
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
ON CONFLICT DO NOTHING;

COMMIT;
