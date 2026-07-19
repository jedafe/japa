-- 000147_seed_admin_view_rbac_nodes.down.sql
-- Reverses 000147_seed_admin_view_rbac_nodes.up.sql.
-- Order matters: remove role_permissions rows first (FK references permissions),
-- then delete the 12 permission nodes we added. Only the 12 keys introduced by the
-- matching up migration are touched; other migrations' nodes are unaffected.

BEGIN;

-- 1. Remove any role grants referencing the 12 permission nodes.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN (
        'admin.users.view',
        'admin.content.view',
        'admin.moderation.view',
        'admin.analytics.view',
        'admin.settings.view',
        'admin.roles.view',
        'admin.audit.view',
        'admin.advertising.view',
        'admin.payments.view',
        'admin.ai.view',
        'admin.feature_flags.view',
        'wallet.admin'
    )
);

-- 2. Delete the 12 permission nodes themselves.
DELETE FROM permissions
WHERE key IN (
    'admin.users.view',
    'admin.content.view',
    'admin.moderation.view',
    'admin.analytics.view',
    'admin.settings.view',
    'admin.roles.view',
    'admin.audit.view',
    'admin.advertising.view',
    'admin.payments.view',
    'admin.ai.view',
    'admin.feature_flags.view',
    'wallet.admin'
);

COMMIT;
