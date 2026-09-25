-- 000488_tax_calculator_rbac_all_tenants.down.sql
-- Reverses 000488_tax_calculator_rbac_all_tenants.up.sql.
-- Removes tax_calculator.manage grants added to admin roles across all
-- tenants, restoring the original 000342 state (super_admin only).

BEGIN;

-- Remove tax_calculator.manage from admin roles (keep super_admin grants
-- from 000342). Target only the admin slug to avoid removing super_admin
-- grants that predated this migration.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND rp.permission_id = p.id
  AND p.key = 'tax_calculator.manage'
  AND ur.slug = 'admin'
  AND ur.tenant_id != '00000000-0000-0000-0000-000000000001';

COMMIT;
