-- 000288_seed_university_field_definitions.down.sql
-- Reverses 000288: removes the university_fields custom field definitions
-- and the group-to-entity-class assignment. The field VALUES
-- (custom_field_values rows referencing these field_ids) are cascade-deleted
-- by the FK ON DELETE CASCADE on custom_field_values.field_id.

BEGIN;

-- Remove the group assignment to the 'space' entity class.
DELETE FROM custom_field_group_assignments
WHERE group_id = '00000000-0000-0000-0000-000000000044'
  AND entity_class = 'space'
  AND entity_id IS NULL;

-- Remove the 15 university field definitions. cascades to custom_field_values.
DELETE FROM custom_fields
WHERE group_id = '00000000-0000-0000-0000-000000000044'
  AND id IN (
    '00000000-0000-0000-0000-000000000090',
    '00000000-0000-0000-0000-000000000091',
    '00000000-0000-0000-0000-000000000092',
    '00000000-0000-0000-0000-000000000093',
    '00000000-0000-0000-0000-000000000094',
    '00000000-0000-0000-0000-000000000095',
    '00000000-0000-0000-0000-000000000096',
    '00000000-0000-0000-0000-000000000097',
    '00000000-0000-0000-0000-000000000098',
    '00000000-0000-0000-0000-000000000099',
    '00000000-0000-0000-0000-000000000100',
    '00000000-0000-0000-0000-000000000101',
    '00000000-0000-0000-0000-000000000102',
    '00000000-0000-0000-0000-000000000103',
    '00000000-0000-0000-0000-000000000104'
  );

COMMIT;
