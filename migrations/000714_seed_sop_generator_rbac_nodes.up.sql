-- 000714_seed_sop_generator_rbac_nodes.up.sql
-- AI SOP Generator RBAC nodes (§110, ADR-121).
-- Nodes: sop_generator.view (all), .generate (member+), .manage (admin/super_admin).

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('sop_generator.view', 'View SOP templates and samples (default: all)'),
    ('sop_generator.generate', 'Generate SOP drafts via the AI Gateway (default: member+, quota-enforced)'),
    ('sop_generator.manage', 'Manage SOP templates and publish samples (default: admin, super_admin)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('admin', 'super_admin')
  AND p.key IN ('sop_generator.view', 'sop_generator.generate', 'sop_generator.manage')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug = 'member'
  AND p.key IN ('sop_generator.view', 'sop_generator.generate')
ON CONFLICT DO NOTHING;

COMMIT;
