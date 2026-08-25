-- 000546_jce_geo_and_nested_comparison_types.down.sql
-- Reverses the nested-comparison / geo comparison type seed.

BEGIN;

-- Remove the new comparison types.
DELETE FROM comparison_type_definitions
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug IN ('countries', 'states', 'cities');

-- Remove the custom fields and field group for geo comparisons.
DELETE FROM custom_fields
WHERE group_id IN (
    SELECT id FROM custom_field_groups
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'geo_fields'
);

DELETE FROM custom_field_groups
WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'geo_fields';

-- Remove nested-comparison config from existing system types.
UPDATE comparison_type_definitions
SET config = config
    - 'allow_nested_comparisons'
    - 'allowed_nested_types'
    || CASE
        WHEN slug = 'universities' THEN '{"item_ref_types": ["university"]}'::jsonb
        WHEN slug = 'cost_of_living' THEN '{"item_ref_types": ["col_city"]}'::jsonb
        WHEN slug = 'banks' THEN '{"item_ref_types": ["organization"]}'::jsonb
        WHEN slug = 'courses' THEN '{"item_ref_types": ["course"]}'::jsonb
        WHEN slug = 'visa-types' THEN '{"item_ref_types": ["visa_route"]}'::jsonb
        WHEN slug = 'mentors' THEN '{"item_ref_types": ["mentor"]}'::jsonb
        ELSE '{}'::jsonb
       END
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND is_system = true;

COMMIT;
