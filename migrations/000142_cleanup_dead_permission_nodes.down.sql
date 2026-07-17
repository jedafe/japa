-- 000142_cleanup_dead_permission_nodes.down.sql
-- Re-adds dead control.* permission nodes.

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('control.ai_budget.manage', 'Manage AI budget settings'),
    ('control.api_keys.manage', 'Manage API keys'),
    ('control.audit_logs.view', 'View audit logs'),
    ('control.cache.manage', 'Manage cache'),
    ('control.feature_flags.manage', 'Manage feature flags'),
    ('control.health.view', 'View health status'),
    ('control.queue.manage', 'Manage task queue'),
    ('control.security.manage', 'Manage security settings'),
    ('control.settings.manage', 'Manage system settings'),
    ('control.tenants.manage', 'Manage tenants')
ON CONFLICT (key) DO NOTHING;

-- Grant to super_admin_role.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin_role'
  AND p.key IN (
    'control.ai_budget.manage', 'control.api_keys.manage',
    'control.audit_logs.view', 'control.cache.manage',
    'control.feature_flags.manage', 'control.health.view',
    'control.queue.manage', 'control.security.manage',
    'control.settings.manage', 'control.tenants.manage'
  )
ON CONFLICT DO NOTHING;

COMMIT;
