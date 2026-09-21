-- 000682_seed_intel_channels_rbac_nodes.down.sql
-- Remove the intel source-channel admin RBAC nodes seeded by 000682.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN (
        'intel.channels.list',
        'intel.channels.view',
        'intel.channels.create',
        'intel.channels.update',
        'intel.channels.delete',
        'intel.channels.crawl',
        'intel.sources.probe',
        'intel.inbound.manage'
    )
);

DELETE FROM permissions
WHERE key IN (
    'intel.channels.list',
    'intel.channels.view',
    'intel.channels.create',
    'intel.channels.update',
    'intel.channels.delete',
    'intel.channels.crawl',
    'intel.sources.probe',
    'intel.inbound.manage'
);

COMMIT;
