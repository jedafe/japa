-- 000545_jce_seed_item_ref_types.down.sql
-- Remove the seeded item_ref_types keys from the system comparison type configs.

UPDATE comparison_type_definitions
SET config = config - 'item_ref_types'
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug IN ('universities', 'cost_of_living', 'banks', 'courses', 'visa-types', 'mentors');
