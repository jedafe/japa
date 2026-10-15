-- 000860_wiki_city_overview_fields.down.sql
-- Removes the six seeded city overview fields from wiki_city_overview.
-- field permissions and custom_field_values cascade/delete explicitly.
-- Toronto's seed values are deleted (they were created by this
-- migration); any values added later through the wiki edit path are
-- removed with the fields, matching the field-deletion convention.

BEGIN;

DELETE FROM custom_field_values v
USING custom_fields f, custom_field_groups g
WHERE v.field_id = f.id
  AND f.group_id = g.id
  AND g.slug = 'wiki_city_overview'
  AND f.key IN ('population', 'province_state', 'timezone', 'official_language', 'popular_places', 'main_airport');

DELETE FROM custom_field_permissions p
USING custom_fields f, custom_field_groups g
WHERE p.field_id = f.id
  AND f.group_id = g.id
  AND g.slug = 'wiki_city_overview'
  AND f.key IN ('population', 'province_state', 'timezone', 'official_language', 'popular_places', 'main_airport');

DELETE FROM custom_fields f
USING custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'wiki_city_overview'
  AND f.key IN ('population', 'province_state', 'timezone', 'official_language', 'popular_places', 'main_airport');

COMMIT;
