-- 000785_col_dimension_source_fields.up.sql
-- Bind cost_of_living comparison dimensions to the wiki city Cost of Living
-- (and Prices topic) custom fields via source_field_id. CompareCities'
-- §80 Tool Surface Hydration overlay resolves community field values
-- (custom_field_values on the city's geo space) by the bound field's key,
-- so approved community edits on /wiki/{city}/cost-of-living reach the
-- calculator matrix. Dimensions with no natural wiki counterpart
-- (rent_1br_outside, avg_net_salary) stay unbound.
--
-- No-ops cleanly when the wiki field groups/fields are absent.
--
-- Ref: .jeos/sections/72-cost-of-living.md
--      .jeos/sections/80-destination-wiki.md (Tool Surface Hydration)

BEGIN;

UPDATE comparison_dimensions cd
SET source_field_id = f.id
FROM (VALUES
	('rent_1br_city_centre',  'wiki_city_cost_of_living',      'rent_1br'),
	('groceries_monthly',     'wiki_city_cost_of_living',      'exp_food'),
	('transport_monthly',     'wiki_city_cost_of_living',      'exp_transportation'),
	('utilities_monthly',     'wiki_city_cost_of_living',      'exp_utilities'),
	('internet_monthly',      'wiki_city_cost_of_living',      'exp_internet'),
	('restaurant_meal_cheap', 'wiki_city_prices_restaurants',  'meal_inexpensive'),
	('gym_monthly',           'wiki_city_prices_lifestyle',    'gym_monthly')
) AS m(dim_slug, group_slug, field_key)
JOIN custom_field_groups g ON g.slug = m.group_slug
JOIN custom_fields f ON f.group_id = g.id AND f.key = m.field_key
WHERE cd.slug = m.dim_slug
  AND cd.deleted_at IS NULL
  AND EXISTS (
	SELECT 1 FROM comparison_items ci
	WHERE ci.content_id = cd.content_id
	  AND ci.type_id = (SELECT id FROM comparison_type_definitions
	                  WHERE slug = 'cost_of_living' AND is_system = true)
  );

COMMIT;
