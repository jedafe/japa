-- 000558_seed_relocation_guide_category.down.sql
-- Reverses 000558: removes the relocation guide category, its field group,
-- assignments, fields, and any values that were stored for those fields.

BEGIN;

DELETE FROM custom_field_group_assignments
WHERE entity_class = 'content_type:guide:relocation'
  AND group_id IN (
      SELECT id FROM custom_field_groups
      WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
        AND slug = 'guide_relocation_fields'
  );

DELETE FROM custom_field_groups
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug = 'guide_relocation_fields';

DELETE FROM custom_field_entity_types
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND value = 'guide_category:relocation';

COMMIT;
