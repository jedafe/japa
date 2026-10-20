-- 000901_wiki_mapping_categories_seed.down.sql
-- Reverses 000901: removes the seeded categories, topic templates, group
-- assignments, and field groups for the new wiki types, new city COL
-- subpages, and the geo/university category expansion.
--
-- custom_field_groups is deleted only when the group is no longer referenced
-- (ON DELETE RESTRICT on categories/templates protects live groups anyway;
-- we delete categories/templates first, then orphaned groups).

BEGIN;

-- ---------------------------------------------------------------------------
-- c) Geo/university expansion: categories, assignments, groups.
-- ---------------------------------------------------------------------------
DELETE FROM custom_field_group_assignments a
USING wiki_type_categories c, space_types st
WHERE a.group_id = c.field_group_id
  AND st.id = c.space_type_id
  AND ((st.slug = 'country'     AND c.slug IN ('safety','jobs','taxes','weather'))
    OR (st.slug = 'state'       AND c.slug IN ('safety','jobs','schools','taxes','weather','banking'))
    OR (st.slug = 'city'        AND c.slug IN ('safety','jobs','schools','transportation','utilities','weather','banking'))
    OR (st.slug = 'university'  AND c.slug IN ('rankings')));

DELETE FROM wiki_type_categories c
USING space_types st
WHERE st.id = c.space_type_id
  AND ((st.slug = 'country'     AND c.slug IN ('safety','jobs','taxes','weather'))
    OR (st.slug = 'state'       AND c.slug IN ('safety','jobs','schools','taxes','weather','banking'))
    OR (st.slug = 'city'        AND c.slug IN ('safety','jobs','schools','transportation','utilities','weather','banking'))
    OR (st.slug = 'university'  AND c.slug IN ('rankings')));

DELETE FROM custom_field_groups
WHERE slug IN (
    'wiki_country_safety','wiki_country_jobs','wiki_country_taxes','wiki_country_weather',
    'wiki_state_safety','wiki_state_jobs','wiki_state_schools','wiki_state_taxes','wiki_state_weather','wiki_state_banking',
    'wiki_city_safety','wiki_city_jobs','wiki_city_schools','wiki_city_transportation','wiki_city_utilities','wiki_city_weather','wiki_city_banking',
    'wiki_university_rankings'
);

-- ---------------------------------------------------------------------------
-- b) City COL topic pages: revert the rollup pages list, drop templates,
--    assignments, and groups.
-- ---------------------------------------------------------------------------
UPDATE wiki_type_categories c
SET rollup = jsonb_set(rollup, '{pages}',
    '["food-groceries-prices","restaurant-prices","transportation-prices","utilities-prices","lifestyle-prices"]'::jsonb)
FROM space_types st
WHERE c.space_type_id = st.id
  AND st.slug = 'city'
  AND c.category = 'cost_of_living'
  AND rollup IS NOT NULL;

DELETE FROM wiki_type_page_templates t
USING wiki_type_categories c, space_types st
WHERE t.category_id = c.id
  AND c.space_type_id = st.id
  AND st.slug = 'city'
  AND t.slug IN ('rent-housing-prices','healthcare-prices','childcare-school-fees');

DELETE FROM custom_field_group_assignments a
USING custom_field_groups g
WHERE a.group_id = g.id
  AND g.slug IN ('wiki_city_prices_rent_housing','wiki_city_prices_healthcare','wiki_city_prices_childcare');

DELETE FROM custom_field_groups
WHERE slug IN ('wiki_city_prices_rent_housing','wiki_city_prices_healthcare','wiki_city_prices_childcare');

-- ---------------------------------------------------------------------------
-- a) New wiki types' categories, assignments, and groups.
-- ---------------------------------------------------------------------------
DELETE FROM custom_field_group_assignments a
USING wiki_type_categories c, space_types st
WHERE a.group_id = c.field_group_id
  AND st.id = c.space_type_id
  AND st.slug IN ('neighborhood','visa_route','student_visa','work_visa','family_visa',
                  'tourist_visa','business_visa','pr_pathway','course','scholarship',
                  'bank','embassy');

DELETE FROM wiki_type_categories c
USING space_types st
WHERE st.id = c.space_type_id
  AND st.slug IN ('neighborhood','visa_route','student_visa','work_visa','family_visa',
                  'tourist_visa','business_visa','pr_pathway','course','scholarship',
                  'bank','embassy');

-- Escaped underscores keep 'wiki\_bank\_%' from matching wiki_banking_*
-- groups (default LIKE escape is backslash).
DELETE FROM custom_field_groups g
WHERE g.slug LIKE 'wiki\_neighborhood\_%'
   OR g.slug LIKE 'wiki\_visa\_route\_%'
   OR g.slug LIKE 'wiki\_student\_visa\_%'
   OR g.slug LIKE 'wiki\_work\_visa\_%'
   OR g.slug LIKE 'wiki\_family\_visa\_%'
   OR g.slug LIKE 'wiki\_tourist\_visa\_%'
   OR g.slug LIKE 'wiki\_business\_visa\_%'
   OR g.slug LIKE 'wiki\_pr\_pathway\_%'
   OR g.slug LIKE 'wiki\_course\_%'
   OR g.slug LIKE 'wiki\_scholarship\_%'
   OR g.slug LIKE 'wiki\_bank\_%'
   OR g.slug LIKE 'wiki\_embassy\_%';

COMMIT;
