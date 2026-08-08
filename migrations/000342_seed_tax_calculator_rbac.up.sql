-- 000342_seed_tax_calculator_rbac.up.sql
-- RBAC nodes for Tax Calculator (§100, ADR-108).
--   tax_calculator.view   — all
--   tax_calculator.manage — admin, super_admin
--   tax_content.review    — admin-assigned qualified reviewer

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('tax_calculator.view',   'View tax obligation guides and treaty data (all users)'),
    ('tax_calculator.manage', 'Manage tax obligation guides and treaty pairs (admin+)'),
    ('tax_content.review',    'Review tax content for HITL publication (qualified reviewer)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key IN ('tax_calculator.manage','tax_content.review')
ON CONFLICT DO NOTHING;

COMMIT;
