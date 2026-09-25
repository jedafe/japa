-- 000563_seed_control_panel_rbac_nodes.up.sql
-- Adds granular RBAC permission nodes for Control Panel infrastructure routes
-- that were only gated by the broad control.panel.access node. These nodes
-- allow delegating queue, cron, cache, security, and audit-log access without
-- granting full control panel access.
--
-- Ref: japaconnect-engineering/sections/21-control-panel.md

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('platform.queue.view',     'View job queue stats and pending tasks'),
    ('platform.queue.manage',   'Retry, discard, and manage queue tasks'),
    ('platform.cron.view',      'View scheduler/cron jobs'),
    ('platform.cron.manage',    'Create, update, delete, trigger, pause, and unpause cron jobs'),
    ('platform.cache.view',     'View cache stats'),
    ('platform.cache.manage',   'Flush cache'),
    ('platform.security.view',  'View security scan results and IP block list'),
    ('platform.security.manage','Trigger security scans and manage IP blocks'),
    ('audit.logs.view',         'View audit logs and filters'),
    ('audit.logs.delete',       'Delete and clear audit logs')
ON CONFLICT (key) DO UPDATE SET description = EXCLUDED.description;

-- Grant all new nodes to super_admin so existing control panel access remains
-- intact. Admin roles can be granted view-only nodes later through the RBAC UI.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin', 'super_admin_role')
  AND p.key IN (
      'platform.queue.view', 'platform.queue.manage',
      'platform.cron.view', 'platform.cron.manage',
      'platform.cache.view', 'platform.cache.manage',
      'platform.security.view', 'platform.security.manage',
      'audit.logs.view', 'audit.logs.delete'
  )
ON CONFLICT DO NOTHING;

COMMIT;
