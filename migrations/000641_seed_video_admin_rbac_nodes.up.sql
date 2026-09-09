-- 000641_seed_video_admin_rbac_nodes.up.sql
-- Add RBAC nodes for admin video sources and import jobs management.

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('admin.video_sources.read', 'View admin video sources list and detail'),
    ('admin.video_sources.manage', 'Update and soft-delete video sources'),
    ('admin.video_import_jobs.read', 'View admin video import jobs list and detail'),
    ('admin.video_import_jobs.manage', 'Retry and manage video import jobs')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key IN (
      'admin.video_sources.read',
      'admin.video_sources.manage',
      'admin.video_import_jobs.read',
      'admin.video_import_jobs.manage'
  )
ON CONFLICT DO NOTHING;

COMMIT;
