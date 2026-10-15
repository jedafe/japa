-- 000859_wiki_city_housing_fields_and_stale_cfv.down.sql
-- Reverse the housing seed and spec wiring. rent_1br's stored values
-- restore to cost-of-living from the housing field (they were relocated,
-- not deleted). The exp_* stale cfv rows deleted by the up migration
-- were unreachable-by-contract superseded values and are NOT restored —
-- after this down, those fields resume manual entry at empty.

BEGIN;

-- 1. Put the relocated rent_1br values back on the cost-of-living field.
INSERT INTO custom_field_values (field_id, entity_type, entity_id, value_json, created_at, updated_at)
SELECT cf.id, v.entity_type, v.entity_id, v.value_json, now(), now()
FROM custom_field_values v
JOIN custom_fields hf ON hf.id = v.field_id AND hf.key = 'rent_1br'
JOIN custom_field_groups hg ON hg.id = hf.group_id AND hg.slug = 'wiki_city_housing'
JOIN custom_fields cf ON cf.key = 'rent_1br'
JOIN custom_field_groups cg ON cg.id = cf.group_id AND cg.slug = 'wiki_city_cost_of_living'
WHERE v.entity_type = 'space'
ON CONFLICT (field_id, entity_type, entity_id) DO NOTHING;

-- 2. Strip the housing-derived specs; monthly_total returns to the
--    17-source itemized spec from 000858.
UPDATE custom_fields f
SET options = f.options - 'computed'
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'wiki_city_cost_of_living'
  AND f.key IN ('rent_1br', 'exp_accommodation');

UPDATE custom_fields f
SET options = COALESCE(f.options, '{}'::jsonb) ||
	'{"computed":{"metric":"sum","sources":[
		{"group":"wiki_city_prices_food_groceries","key":"milk_1l"},
		{"group":"wiki_city_prices_food_groceries","key":"bread_500g"},
		{"group":"wiki_city_prices_food_groceries","key":"rice_1kg"},
		{"group":"wiki_city_prices_food_groceries","key":"eggs_12"},
		{"group":"wiki_city_prices_food_groceries","key":"chicken_1kg"},
		{"group":"wiki_city_prices_food_groceries","key":"beef_1kg"},
		{"group":"wiki_city_prices_food_groceries","key":"apples_1kg"},
		{"group":"wiki_city_prices_food_groceries","key":"bananas_1kg"},
		{"group":"wiki_city_prices_food_groceries","key":"tomatoes_1kg"},
		{"group":"wiki_city_prices_food_groceries","key":"potatoes_1kg"},
		{"group":"wiki_city_prices_transportation","key":"ttc_monthly_adult"},
		{"group":"wiki_city_prices_utilities","key":"utilities_85sqm"},
		{"group":"wiki_city_prices_utilities","key":"internet_60"},
		{"group":"wiki_city_prices_utilities","key":"mobile_10gb"},
		{"group":"wiki_city_prices_lifestyle","key":"gym_monthly"},
		{"group":"wiki_city_prices_lifestyle","key":"movie_ticket"},
		{"group":"wiki_city_prices_lifestyle","key":"netflix_standard"}]}}'::jsonb
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'wiki_city_cost_of_living'
  AND f.key = 'monthly_total';

-- 3. Undo the item_ref self-reference backfill — only rows still
--    carrying exactly the shape we wrote are touched.
UPDATE comparison_items i
SET item_ref = '{}'::jsonb
FROM comparison_type_definitions t
WHERE i.type_id = t.id
  AND t.slug = 'cost_of_living'
  AND i.item_ref = jsonb_build_object('type', 'col_city', 'id', i.id::text);

-- 4. Remove the seeded housing field data (cfv cascade via field_id).
DELETE FROM custom_field_values v
USING custom_fields f, custom_field_groups g
WHERE v.field_id = f.id
  AND f.group_id = g.id
  AND g.slug = 'wiki_city_housing'
  AND f.key IN ('rent_1br', 'rent_3br', 'buy_sqm_centre', 'buy_sqm_outside');

DELETE FROM custom_field_permissions p
USING custom_fields f, custom_field_groups g
WHERE p.field_id = f.id
  AND f.group_id = g.id
  AND g.slug = 'wiki_city_housing'
  AND f.key IN ('rent_1br', 'rent_3br', 'buy_sqm_centre', 'buy_sqm_outside');

DELETE FROM custom_fields f
USING custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'wiki_city_housing'
  AND f.key IN ('rent_1br', 'rent_3br', 'buy_sqm_centre', 'buy_sqm_outside');

COMMIT;
