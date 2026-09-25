-- 000642_fix_video_admin_rbac_tenants.down.sql
-- Remove the cross-tenant video admin grants from super_admin roles.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN (
        'admin.video_claims.read',
        'admin.video_claims.manage',
        'admin.video_sources.read',
        'admin.video_sources.manage',
        'admin.video_import_jobs.read',
        'admin.video_import_jobs.manage'
    )
)
AND role_id IN (
    SELECT id FROM user_roles WHERE slug = 'super_admin'
);

COMMIT;
