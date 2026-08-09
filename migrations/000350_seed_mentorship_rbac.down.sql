-- 000350_seed_mentorship_rbac.down.sql
-- Rollback: remove mentorship RBAC nodes and role grants.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN ('mentorship.become_mentor', 'mentorship.request',
                  'mentorship.respond', 'mentorship.manage')
);

DELETE FROM permissions
WHERE key IN ('mentorship.become_mentor', 'mentorship.request',
              'mentorship.respond', 'mentorship.manage');

COMMIT;
