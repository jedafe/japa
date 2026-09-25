-- 000252_jobs_board.down.sql
-- Reverses 000252_jobs_board.up.sql.

BEGIN;

-- Revoke jobs.* grants.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin', 'member')
  AND rp.permission_id = p.id
  AND p.key IN ('jobs.browse', 'jobs.apply', 'jobs.post', 'jobs.manage');

-- Remove jobs.* permission nodes.
DELETE FROM permissions WHERE key IN ('jobs.browse', 'jobs.apply', 'jobs.post', 'jobs.manage');

-- Remove jobs.* system_settings.
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('jobs.enabled', 'jobs.featured_credits');

-- Drop the job_applications table.
DROP TABLE IF EXISTS job_applications;

COMMIT;
