-- 001040_region_wiki_fields.down.sql — drop region field schemas + perms.
BEGIN;

DELETE FROM custom_field_permissions p
USING custom_fields f, custom_field_groups g
WHERE p.field_id = f.id AND f.group_id = g.id
  AND g.slug IN ('wiki_region_overview','wiki_region_cost_of_living','wiki_region_other');

DELETE FROM custom_fields f
USING custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug IN ('wiki_region_overview','wiki_region_cost_of_living','wiki_region_other');

COMMIT;
