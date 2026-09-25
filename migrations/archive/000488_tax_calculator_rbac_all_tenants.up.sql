-- 000488_tax_calculator_rbac_all_tenants.up.sql
-- BUG-005: The original tax calculator RBAC seed (000342) only granted
-- tax_calculator.manage and tax_content.review to super_admin in the
-- DEFAULT tenant (00000000-0000-0000-0000-000000000001). §100 +
-- permission-key-naming.md specify:
--   tax_calculator.manage → admin, super_admin
-- Admins were locked out of the manage routes (403). This migration
-- grants tax_calculator.manage to admin + super_admin across ALL tenants.
-- tax_content.review remains super_admin-only (admin-assigned to a
-- qualified reviewer per §100/ADR-108).
--
-- Ref: japaconnect-engineering/sections/100-tax-calculator.md
-- Ref: japaconnect-engineering/adr/ADR-108-tax-calculator.md
-- Ref: japaconnect-engineering/conventions/permission-key-naming.md
-- Pattern: 000485_scam_alert_rbac_all_tenants.up.sql

BEGIN;

-- tax_calculator.manage — admin + super_admin in all tenants.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'tax_calculator.manage'
  AND ur.slug IN ('admin', 'super_admin')
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp
      WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

COMMIT;
