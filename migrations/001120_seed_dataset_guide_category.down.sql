-- 001120_seed_dataset_guide_category.down.sql
-- Removes the dataset guide category seed. The 001117 delete guard will
-- reject the entity-type delete if any live contents still carry
-- guide_category='dataset' — clear or delete those rows first.

BEGIN;

DELETE FROM custom_fields f
USING custom_field_groups g
WHERE f.group_id = g.id AND g.slug = 'guide_dataset_fields';

DELETE FROM custom_field_group_assignments a
USING custom_field_groups g
WHERE a.group_id = g.id AND g.slug = 'guide_dataset_fields';

DELETE FROM custom_field_groups WHERE slug = 'guide_dataset_fields';

DELETE FROM custom_field_entity_types WHERE value = 'guide_category:dataset';

COMMIT;
