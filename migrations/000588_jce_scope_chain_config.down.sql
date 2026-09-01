-- Remove scope_chain from the affected JCE system comparison type configs.
-- Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
BEGIN;

UPDATE comparison_type_definitions
SET config = config - 'scope_chain'
WHERE slug IN ('cost_of_living', 'universities', 'countries', 'states', 'cities', 'courses', 'visa-types')
  AND is_system = true
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

COMMIT;
