-- 000712_seed_pof_calculator_rbac_nodes.up.sql
-- POF Calculator RBAC nodes (§108, ADR-119).
-- Nodes: pof_calculator.view (all), .manage (admin/super_admin).

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('pof_calculator.view', 'View the Proof of Funds Calculator tool (default: all)'),
    ('pof_calculator.manage', 'Manage POF requirement figures (default: admin, super_admin)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('admin', 'super_admin')
  AND p.key IN ('pof_calculator.view', 'pof_calculator.manage')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug = 'member'
  AND p.key = 'pof_calculator.view'
ON CONFLICT DO NOTHING;

COMMIT;
