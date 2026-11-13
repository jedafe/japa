-- Remove the cost-of-living rollup groups from the generic geo type
-- pools. COL rollups are dedicated types (country/state/city-cost-of-
-- living); the same group inside a generic countries/states/cities
-- comparison duplicates them, and price-dimension groups were already
-- kept exclusive. Wiki-page COL sections (wiki_type_categories) are
-- untouched — this only trims the comparison picker rail.

BEGIN;

DELETE FROM comparison_type_field_groups ctfg
USING comparison_type_definitions d, custom_field_groups g
WHERE ctfg.type_id = d.id
  AND ctfg.group_id = g.id
  AND (
    (d.slug = 'cities'    AND g.slug = 'wiki_city_cost_of_living')
    OR (d.slug = 'countries' AND g.slug = 'wiki_country_cost_of_living')
    OR (d.slug = 'states'    AND g.slug = 'wiki_state_cost_of_living')
  );

COMMIT;
