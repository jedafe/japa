-- 000642_fix_video_admin_rbac_tenants.up.sql
-- Re-grant all video admin RBAC nodes to every super_admin role, regardless of
-- tenant. Earlier seed migrations (000638 and 000641) were scoped to the default
-- tenant only; this migration ensures multi-tenant super_admin roles also get
-- video claim, source, and import-job permissions.

BEGIN;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug = 'super_admin'
  AND p.key IN (
      'admin.video_claims.read',
      'admin.video_claims.manage',
      'admin.video_sources.read',
      'admin.video_sources.manage',
      'admin.video_import_jobs.read',
      'admin.video_import_jobs.manage'
  )
ON CONFLICT DO NOTHING;

COMMIT;
