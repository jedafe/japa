-- 000713_seed_eca_guide_rbac_nodes.up.sql
-- ECA Guide RBAC nodes (§109, ADR-120).
-- Nodes: eca_guide.view (all), .manage (admin/super_admin), .update (member+).

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('eca_guide.view', 'View the ECA/WES Evaluation Guide tool (default: all)'),
    ('eca_guide.manage', 'Manage ECA providers, guides, and community updates (default: admin, super_admin)'),
    ('eca_guide.update', 'Submit community fee/timeline updates to ECA guides (default: member+)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('admin', 'super_admin')
  AND p.key IN ('eca_guide.view', 'eca_guide.manage', 'eca_guide.update')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug = 'member'
  AND p.key IN ('eca_guide.view', 'eca_guide.update')
ON CONFLICT DO NOTHING;

COMMIT;
