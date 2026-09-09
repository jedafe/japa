-- 000641_seed_video_admin_rbac_nodes.down.sql
-- Remove admin video sources and import jobs RBAC nodes.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN (
        'admin.video_sources.read',
        'admin.video_sources.manage',
        'admin.video_import_jobs.read',
        'admin.video_import_jobs.manage'
    )
);

DELETE FROM permissions
WHERE key IN (
    'admin.video_sources.read',
    'admin.video_sources.manage',
    'admin.video_import_jobs.read',
    'admin.video_import_jobs.manage'
);

COMMIT;
