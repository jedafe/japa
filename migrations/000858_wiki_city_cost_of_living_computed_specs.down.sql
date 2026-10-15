-- 000858_wiki_city_cost_of_living_computed_specs.down.sql
-- Strip options.computed from the seeded cost-of-living fields — manual
-- cfv entry resumes (stored rows were never touched by the read path).

BEGIN;

UPDATE custom_fields f
SET options = f.options - 'computed'
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'wiki_city_cost_of_living'
  AND f.key IN ('exp_food', 'exp_transportation', 'exp_utilities',
                'exp_internet', 'exp_mobile', 'exp_entertainment',
                'monthly_total');

UPDATE wiki_type_categories c
SET rollup = c.rollup - 'include_self'
FROM space_types st
WHERE c.space_type_id = st.id
  AND st.slug = 'city'
  AND c.category = 'cost_of_living'
  AND c.rollup ? 'mode'
  AND c.rollup->>'mode' = 'pages';

COMMIT;
