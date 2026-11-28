-- 001239_mentor_applications.down.sql
-- Reverses 001239: notification types, the mentors group + backing role,
-- the mentorship.apply node, then the table (drops first so no FKs linger
-- on users; group removal runs after membership cleanup).

BEGIN;

DELETE FROM notification_types
WHERE key IN ('mentorship_application_approved', 'mentorship_application_declined');

-- Members of the mentors group lose membership before the group goes.
DELETE FROM user_group_memberships
WHERE group_id IN (SELECT id FROM user_groups WHERE slug = 'mentors');

DELETE FROM group_roles
WHERE group_id IN (SELECT id FROM user_groups WHERE slug = 'mentors')
   OR role_id IN (SELECT id FROM user_roles WHERE slug = 'mentor-role');

DELETE FROM user_groups WHERE slug = 'mentors';

DELETE FROM role_permissions
WHERE role_id IN (SELECT id FROM user_roles WHERE slug = 'mentor-role');

DELETE FROM user_roles WHERE slug = 'mentor-role';

DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE key = 'mentorship.apply');

DELETE FROM permissions WHERE key = 'mentorship.apply';

DROP TABLE IF EXISTS mentor_applications;

COMMIT;
