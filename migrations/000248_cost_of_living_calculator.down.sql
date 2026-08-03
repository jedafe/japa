-- 000248_cost_of_living_calculator.down.sql
-- Reverses 000248_cost_of_living_calculator.up.sql.

BEGIN;

-- Revoke col_calculator.* grants.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin', 'member')
  AND rp.permission_id = p.id
  AND p.key IN ('col_calculator.manage', 'col_calculator.use');

-- Remove col_calculator.* permission nodes.
DELETE FROM permissions WHERE key IN ('col_calculator.use', 'col_calculator.manage');

-- Remove col_calculator.* system_settings.
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('col_calculator.enabled', 'col_calculator.stale_days', 'col_calculator.max_cities');

-- Remove the seeded cost_of_living comparison type.
DELETE FROM comparison_type_definitions
WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'cost_of_living';

-- Drop the added columns.
ALTER TABLE comparison_type_definitions DROP COLUMN IF EXISTS config;
ALTER TABLE comparison_items DROP COLUMN IF EXISTS last_updated_at;
ALTER TABLE comparison_items DROP COLUMN IF EXISTS is_baseline;

DROP INDEX IF EXISTS idx_comparison_items_baseline;

COMMIT;
