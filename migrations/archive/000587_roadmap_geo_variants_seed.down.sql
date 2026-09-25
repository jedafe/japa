-- 000587_roadmap_geo_variants_seed.down.sql
-- Remove all placeholder roadmap variants created by the up migration.

BEGIN;

DELETE FROM contents
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND guide_category = 'roadmap'
  AND status = 'draft'
  AND field_values->>'placeholder_variant' = 'true';

COMMIT;
