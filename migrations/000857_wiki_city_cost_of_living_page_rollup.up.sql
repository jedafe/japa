-- 000857_wiki_city_cost_of_living_page_rollup.up.sql
-- §80 page rollup amendment: the city's cost_of_living sub-pages feed
-- the category page's aggregates directly, so the 'prices' intermediate
-- level is redundant. Flatten the topic templates to direct children of
-- the category, drop the 'prices' template, and switch the city
-- cost_of_living rollup to mode "pages".
--
-- The wiki_city_prices group and the 'city-prices' comparison type are
-- untouched — the flat city↔city comparison keeps working. Materialized
-- /wiki/{city}/prices rows survive as ad-hoc pages bound to that group
-- (page_template_id SET NULL); unmaterialized lazy /prices URLs 404.
--
-- Ref: docs/superpowers/specs/2026-10-15-wiki-page-rollup-computed-fields-design.md

BEGIN;

UPDATE wiki_type_page_templates t
SET parent_template_id = NULL
FROM wiki_type_categories c
JOIN space_types st ON st.id = c.space_type_id
WHERE t.category_id = c.id
  AND st.slug = 'city'
  AND c.category = 'cost_of_living'
  AND t.slug IN ('food-groceries-prices', 'restaurant-prices',
                 'transportation-prices', 'utilities-prices',
                 'lifestyle-prices');

DELETE FROM wiki_type_page_templates t
USING wiki_type_categories c
JOIN space_types st ON st.id = c.space_type_id
WHERE t.category_id = c.id
  AND st.slug = 'city'
  AND c.category = 'cost_of_living'
  AND t.slug = 'prices'
  AND t.is_system;

UPDATE wiki_type_categories c
SET rollup = '{"enabled": true, "mode": "pages",
               "metrics": ["min", "avg", "max"],
               "pages": ["food-groceries-prices", "restaurant-prices",
                          "transportation-prices", "utilities-prices",
                          "lifestyle-prices"]}'::jsonb
FROM space_types st
WHERE c.space_type_id = st.id
  AND st.slug = 'city'
  AND c.category = 'cost_of_living';

COMMIT;
