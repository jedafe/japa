-- 000785_col_dimension_source_fields.down.sql
-- Revert 000785: drop the wiki-field source bindings on cost_of_living
-- comparison dimensions.

BEGIN;

UPDATE comparison_dimensions cd
SET source_field_id = NULL
FROM comparison_items ci
WHERE ci.content_id = cd.content_id
  AND ci.type_id = (SELECT id FROM comparison_type_definitions
                    WHERE slug = 'cost_of_living' AND is_system = true)
  AND cd.slug IN (
	'rent_1br_city_centre',
	'groceries_monthly',
	'transport_monthly',
	'utilities_monthly',
	'internet_monthly',
	'restaurant_meal_cheap',
	'gym_monthly'
  )
  AND cd.deleted_at IS NULL;

COMMIT;
