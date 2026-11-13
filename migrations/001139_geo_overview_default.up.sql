-- Make wiki_*_overview the default field group for the geo pickers.
-- geo_fields carried is_default=true until 001138 removed its pool
-- rows, leaving cities/countries/states with nothing pre-selected.
-- Every other type marks exactly one pool row as default (usually its
-- overview) — this restores that convention on the consolidated group.

BEGIN;

UPDATE comparison_type_field_groups cfg
SET is_default = false
WHERE cfg.type_id IN (SELECT id FROM comparison_type_definitions WHERE slug IN ('cities', 'countries', 'states'));

UPDATE comparison_type_field_groups cfg
SET is_default = true
FROM custom_field_groups g, comparison_type_definitions t
WHERE cfg.group_id = g.id AND cfg.type_id = t.id
  AND ((t.slug = 'cities'    AND g.slug = 'wiki_city_overview')
    OR (t.slug = 'countries' AND g.slug = 'wiki_country_overview')
    OR (t.slug = 'states'    AND g.slug = 'wiki_state_overview'));

COMMIT;
