-- 000563_seed_control_panel_rbac_nodes.down.sql
-- Removes the granular Control Panel infrastructure RBAC nodes added in the
-- up migration and their grants from super_admin roles.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN (
        'platform.queue.view', 'platform.queue.manage',
        'platform.cron.view', 'platform.cron.manage',
        'platform.cache.view', 'platform.cache.manage',
        'platform.security.view', 'platform.security.manage',
        'audit.logs.view', 'audit.logs.delete'
    )
);

DELETE FROM permissions WHERE key IN (
    'platform.queue.view', 'platform.queue.manage',
    'platform.cron.view', 'platform.cron.manage',
    'platform.cache.view', 'platform.cache.manage',
    'platform.security.view', 'platform.security.manage',
    'audit.logs.view', 'audit.logs.delete'
);

COMMIT;
