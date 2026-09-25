-- 000678_seed_internship_import_export_guide_categories.down.sql
-- Reverses 000678: removes the internship and import/export guide
-- categories, their field groups, assignments, fields (via cascade),
-- and entity type rows.

BEGIN;

DELETE FROM custom_field_group_assignments
WHERE entity_class IN ('content_type:guide:internship', 'content_type:guide:import_export')
  AND group_id IN (
      SELECT id FROM custom_field_groups
      WHERE slug IN ('guide_internship_fields', 'guide_import_export_fields')
  );

DELETE FROM custom_field_groups
WHERE slug IN ('guide_internship_fields', 'guide_import_export_fields');

DELETE FROM custom_field_entity_types
WHERE value IN ('guide_category:internship', 'guide_category:import_export');

COMMIT;
