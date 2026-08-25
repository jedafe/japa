-- 000545_jce_seed_item_ref_types.up.sql
-- Backfill comparison type config with item_ref_types so the JCE item picker
-- can resolve the correct entity resolver for each system comparison type.
--
-- Ref: docs/superpowers/specs/2026-08-22-jce-subspec-1-design.md (§10.1)

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || '{"item_ref_types": ["university"]}'::jsonb
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug = 'universities';

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || '{"item_ref_types": ["col_city"]}'::jsonb
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug = 'cost_of_living';

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || '{"item_ref_types": ["organization"]}'::jsonb
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug = 'banks';

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || '{"item_ref_types": ["course"]}'::jsonb
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug = 'courses';

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || '{"item_ref_types": ["visa_route"]}'::jsonb
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug = 'visa-types';

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || '{"item_ref_types": ["mentor"]}'::jsonb
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug = 'mentors';
