-- 000739_wiki_types_categories.down.sql
-- Reverse the wiki type/category template tables.
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md

BEGIN;

-- Remove assignments + groups bound to category templates before dropping
-- the referencing table. Note: if an admin added fields/values to a seeded
-- group since migrate-up, the custom_field_groups delete will fail on the
-- FK — clear the group's fields/values first (same in_use contract as
-- DeleteFieldGroup).
DELETE FROM custom_field_group_assignments a
USING wiki_type_categories wtc
WHERE a.group_id = wtc.field_group_id;

DELETE FROM custom_field_groups g
USING wiki_type_categories wtc
WHERE g.id = wtc.field_group_id;

DROP TABLE IF EXISTS wiki_type_categories;
DROP TABLE IF EXISTS wiki_type_parents;
DROP TABLE IF EXISTS wiki_type_configs;

COMMIT;
