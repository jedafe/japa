-- 000869_wiki_col_currency_strip.up.sql
-- §58 fix: the wiki cost-of-living field schemas were seeded with
-- options.currency_code = "CAD" (db/seed/wiki_example_cost_of_living.sql
-- + the 000859 housing fields). A field-declared code wins over the host
-- space's resolved country currency, so every city rendered CAD.
-- These fields hold LOCAL prices — they must not declare a code so the
-- §58 country→currency linkage denominates them per space.
-- qf_currency gains {"default_source":"space_currency"}: empty values
-- auto-render the space's resolved currency name (read-time, overridable).
BEGIN;

UPDATE custom_fields f
SET options = f.options - 'currency_code'
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug IN (
    'wiki_city_cost_of_living', 'wiki_country_cost_of_living',
    'wiki_city_prices', 'wiki_city_housing',
    'wiki_city_prices_food_groceries', 'wiki_city_prices_restaurants',
    'wiki_city_prices_utilities', 'wiki_city_prices_lifestyle',
    'wiki_city_prices_transportation'
  )
  AND f.options ? 'currency_code';

UPDATE custom_fields f
SET options = COALESCE(f.options, '{}'::jsonb) || '{"default_source":"space_currency"}'::jsonb
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug LIKE 'wiki\_%'
  AND f.key = 'qf_currency'
  AND f.field_type = 'text';

COMMIT;
