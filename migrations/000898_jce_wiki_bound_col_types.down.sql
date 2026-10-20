-- 000898_jce_wiki_bound_col_types.down.sql
-- Reverses 000898: removes the seeded wiki-bound COL comparison types and
-- strips the family/level keys merged into existing type configs.

BEGIN;

DELETE FROM comparison_type_definitions
WHERE slug IN (
    'country-cost-of-living',
    'state-cost-of-living',
    'city-cost-of-living',
    'col-transportation',
    'col-restaurants',
    'col-food-groceries',
    'col-utilities',
    'col-lifestyle'
);

UPDATE comparison_type_definitions
SET config = config - 'family' - 'level'
WHERE slug IN ('countries', 'states', 'cities', 'universities', 'courses');

UPDATE comparison_type_definitions
SET config = config - 'family' - 'level'
WHERE slug IN ('banks', 'visa-types');

COMMIT;
