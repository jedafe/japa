-- 000739_wiki_types_categories.down.sql
-- Reverse the wiki type/category template tables.
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md

BEGIN;

-- field_group_id is ON DELETE RESTRICT, so the groups cannot be deleted
-- while category rows still reference them. Capture the bound ids first,
-- drop the tables, then delete the groups that follow the auto-provisioned
-- wiki_{type}_{category} naming convention — a category rebound to a
-- pre-existing shared group must not destroy that group on rollback.
CREATE TEMP TABLE _wtc_groups AS
SELECT field_group_id FROM wiki_type_categories;

DROP TABLE IF EXISTS wiki_type_categories;
DROP TABLE IF EXISTS wiki_type_parents;
DROP TABLE IF EXISTS wiki_type_configs;

DELETE FROM custom_field_group_assignments a
USING custom_field_groups g
WHERE a.group_id = g.id
  AND g.id IN (SELECT field_group_id FROM _wtc_groups)
  AND g.slug LIKE 'wiki\_%';

-- Note: if an admin added fields/values to a seeded group since
-- migrate-up, this delete fails on the custom_fields FK — clear the
-- group's fields/values first (same in_use contract as DeleteFieldGroup).
DELETE FROM custom_field_groups g
WHERE g.id IN (SELECT field_group_id FROM _wtc_groups)
  AND g.slug LIKE 'wiki\_%';

DROP TABLE _wtc_groups;

COMMIT;
