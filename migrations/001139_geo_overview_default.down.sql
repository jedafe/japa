-- Revert overview defaults on the geo pools (matches the post-001138
-- state where no row was default).

BEGIN;

UPDATE comparison_type_field_groups cfg
SET is_default = false
FROM custom_field_groups g, comparison_type_definitions t
WHERE cfg.group_id = g.id AND cfg.type_id = t.id
  AND g.slug IN ('wiki_city_overview', 'wiki_country_overview', 'wiki_state_overview')
  AND t.slug IN ('cities', 'countries', 'states');

COMMIT;
