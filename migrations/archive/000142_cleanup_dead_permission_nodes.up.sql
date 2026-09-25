-- 000142_cleanup_dead_permission_nodes.up.sql
-- Removes dead control.* permission nodes that are never middleware-checked.
-- The control panel gates on control.panel.access only — per-route nodes
-- were cataloged but never wired. Also removes legacy dot-scoped duplicates
-- that were superseded by underscore-scoped equivalents.

BEGIN;

-- Remove dead control.* nodes (control.panel.access is kept — it's the
-- only control node actually checked by RequirePermission).
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN (
        'control.ai_budget.manage', 'control.api_keys.manage',
        'control.audit_logs.view', 'control.cache.manage',
        'control.feature_flags.manage', 'control.health.view',
        'control.queue.manage', 'control.security.manage',
        'control.settings.manage', 'control.tenants.manage'
    )
);
DELETE FROM permissions WHERE key IN (
    'control.ai_budget.manage', 'control.api_keys.manage',
    'control.audit_logs.view', 'control.cache.manage',
    'control.feature_flags.manage', 'control.health.view',
    'control.queue.manage', 'control.security.manage',
    'control.settings.manage', 'control.tenants.manage'
);

-- Remove legacy dot-scoped duplicates (000135 already removed them from
-- role_permissions; this cleans up any fresh-deploy artifacts).
DELETE FROM permissions WHERE key IN (
    'blog.edit.any', 'blog.edit.own',
    'content.delete.any', 'content.delete.own',
    'content.edit.any', 'content.edit.own',
    'media.delete.any', 'media.delete.own'
);

COMMIT;
