-- 000294_seed_db_health_rbac_nodes.up.sql
-- Seeds the platform.db.health_view and platform.db.vacuum_trigger permission
-- nodes (specified in Section 21 but never seeded) and grants them to
-- super_admin so the DB health and vacuum routes can be properly gated.
--
-- Ref: japaconnect-engineering/sections/21-control-panel.md (Database Health Tooling)
--      japaconnect-engineering/conventions/permission-key-naming.md
--
-- Context: Section 21 L239-240 defines two permission nodes for the DB health
-- panel: platform.db.health_view (view bloat, queries) and
-- platform.db.vacuum_trigger (trigger manual VACUUM task). These nodes were
-- never seeded into the permissions table, so no role — including super_admin —
-- could hold them. The routes at /control/health/db and
-- /control/health/db/vacuum were only gated by control.panel.access at the
-- group level. This migration seeds both nodes and grants them to super_admin.

BEGIN;

-- 1. Seed the two DB health permission nodes.
INSERT INTO permissions (key, description)
VALUES
    ('platform.db.health_view', 'View DB health detail (bloat, dead tuples, slow queries)'),
    ('platform.db.vacuum_trigger', 'Trigger manual VACUUM task')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant both nodes to super_admin.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key IN ('platform.db.health_view', 'platform.db.vacuum_trigger')
ON CONFLICT DO NOTHING;

COMMIT;
