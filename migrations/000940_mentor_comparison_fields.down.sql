-- 000940_mentor_comparison_fields.down.sql
-- Restore the mentors comparison type to the universities field group and
-- drop the mentor_fields group (its fields cascade via group_id FK).

BEGIN;

UPDATE comparison_type_definitions t
SET field_group_id = g.id
FROM custom_field_groups g
WHERE g.slug = 'university_fields'
  AND t.slug = 'mentors';

-- comparison_dimensions.source_field_id has no cascade — unbind any
-- dimensions that referenced mentor fields before dropping the group.
UPDATE comparison_dimensions d
SET source_field_id = NULL
FROM custom_fields f
JOIN custom_field_groups g ON g.id = f.group_id AND g.slug = 'mentor_fields'
WHERE d.source_field_id = f.id;

DELETE FROM custom_field_groups WHERE slug = 'mentor_fields';

COMMIT;
