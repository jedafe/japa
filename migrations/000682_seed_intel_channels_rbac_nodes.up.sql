-- 000682_seed_intel_channels_rbac_nodes.up.sql
-- Add RBAC nodes for the intel source-channel admin endpoints
-- (intel-channels Phase 1, spec §5). intel.inbound.manage is seeded now for
-- the Phase 3 inbound-events admin surface (no routes consume it yet).

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('intel.channels.list',   'Intel Channels List'),
    ('intel.channels.view',   'Intel Channels View'),
    ('intel.channels.create', 'Intel Channels Create'),
    ('intel.channels.update', 'Intel Channels Update'),
    ('intel.channels.delete', 'Intel Channels Delete'),
    ('intel.channels.crawl',  'Intel Channels Crawl Now'),
    ('intel.sources.probe',   'Intel Sources Re-Probe'),
    ('intel.inbound.manage',  'Intel Inbound Events Manage')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug = 'super_admin'
  AND p.key IN (
      'intel.channels.list',
      'intel.channels.view',
      'intel.channels.create',
      'intel.channels.update',
      'intel.channels.delete',
      'intel.channels.crawl',
      'intel.sources.probe',
      'intel.inbound.manage'
  )
ON CONFLICT DO NOTHING;

COMMIT;
