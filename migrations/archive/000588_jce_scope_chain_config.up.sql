-- Add scope_chain to system comparison type configs for the JCE geo-scoped picker.
-- Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
BEGIN;

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object('scope_chain', jsonb_build_array('country', 'state', 'city'))
WHERE slug IN ('cost_of_living', 'universities', 'cities') AND is_system = true
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object('scope_chain', jsonb_build_array('region', 'subregion'))
WHERE slug = 'countries' AND is_system = true
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object('scope_chain', jsonb_build_array('country', 'state'))
WHERE slug = 'states' AND is_system = true
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object('scope_chain', jsonb_build_array('university'))
WHERE slug = 'courses' AND is_system = true
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object('scope_chain', jsonb_build_array('country'))
WHERE slug = 'visa-types' AND is_system = true
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

COMMIT;
