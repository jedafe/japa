-- Restore geo_fields pool rows, the official_languag field, and drop
-- the copied overview fields.

BEGIN;

INSERT INTO comparison_type_field_groups (type_id, group_id, position, category, label, is_default)
SELECT t.id, '4dfd79c2-d087-4e5c-80b1-bb10b5f90b89', -1, NULL, 'Geo Fields', true
FROM comparison_type_definitions t WHERE t.slug IN ('cities', 'countries', 'states');

INSERT INTO custom_fields (id, group_id, label, key, field_type, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position, config, section)
VALUES ('04b8b098-2822-4682-bf32-326ab00d309b', '7b603a15-0835-4cf3-a209-2393e3cbb858', 'Official Language', 'official_languag', 'text', false, false, true, false, false, false, 0, '{}', 'basic_information');

DELETE FROM custom_fields
WHERE group_id = 'de0d26b4-3f05-47e0-961f-f8755a769b79'
  AND key IN ('area', 'capital', 'language', 'currency', 'postal_codes', 'calling_code', 'official_website', 'founded', 'head_of_government');

DELETE FROM custom_fields
WHERE group_id = '7b603a15-0835-4cf3-a209-2393e3cbb858'
  AND key IN ('area', 'postal_codes', 'official_website', 'founded', 'head_of_government');

DELETE FROM custom_fields
WHERE group_id = 'a9d25f02-2380-40a2-8075-96efe8b678ba'
  AND key IN ('area', 'currency', 'postal_codes', 'calling_code', 'official_website', 'founded', 'head_of_government');

COMMIT;
