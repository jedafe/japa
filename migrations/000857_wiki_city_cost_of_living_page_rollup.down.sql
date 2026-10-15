-- 000857_wiki_city_cost_of_living_page_rollup.down.sql
-- Restore the 000750/000775 shape: 'prices' template re-inserted, topic
-- templates re-nested under it, city cost_of_living rollup cleared.

BEGIN;

-- Restore the 'prices' template (000750 seed shape).
INSERT INTO wiki_type_page_templates (category_id, slug, label, field_group_id, position, is_system)
SELECT c.id, 'prices', 'Prices in {name}', g.id, 1, true
FROM wiki_type_categories c
JOIN space_types st ON st.id = c.space_type_id AND st.slug = 'city'
JOIN custom_field_groups g ON g.slug = 'wiki_city_prices'
WHERE c.category = 'cost_of_living'
ON CONFLICT (category_id, slug) DO NOTHING;

-- Re-nest the topic templates under 'prices'.
UPDATE wiki_type_page_templates t
SET parent_template_id = p.id
FROM wiki_type_categories c
JOIN space_types st ON st.id = c.space_type_id
JOIN wiki_type_page_templates p
  ON p.category_id = c.id AND p.slug = 'prices'
WHERE t.category_id = c.id
  AND st.slug = 'city'
  AND c.category = 'cost_of_living'
  AND t.slug IN ('food-groceries-prices', 'restaurant-prices',
                 'transportation-prices', 'utilities-prices',
                 'lifestyle-prices');

UPDATE wiki_type_categories c
SET rollup = NULL
FROM space_types st
WHERE c.space_type_id = st.id
  AND st.slug = 'city'
  AND c.category = 'cost_of_living';

COMMIT;
