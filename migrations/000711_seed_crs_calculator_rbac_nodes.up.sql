-- 000711_seed_crs_calculator_rbac_nodes.up.sql
-- CRS Calculator RBAC nodes (§107, ADR-118).
-- Nodes: crs_calculator.view (all), .manage (admin/super_admin), .history (member+).

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('crs_calculator.view', 'View the Express Entry CRS Calculator tool (default: all)'),
    ('crs_calculator.manage', 'Manage CRS rulesets and draw entries (default: admin, super_admin)'),
    ('crs_calculator.history', 'Save and view personal CRS score history (default: member+)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('admin', 'super_admin')
  AND p.key IN ('crs_calculator.view', 'crs_calculator.manage', 'crs_calculator.history')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug = 'member'
  AND p.key IN ('crs_calculator.view', 'crs_calculator.history')
ON CONFLICT DO NOTHING;

COMMIT;
