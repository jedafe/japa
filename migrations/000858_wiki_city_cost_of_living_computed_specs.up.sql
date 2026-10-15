-- 000858_wiki_city_cost_of_living_computed_specs.up.sql
-- §80 page rollup amendment: seed options.computed on the city
-- cost-of-living summary fields so they derive from the topic groups'
-- itemized values instead of manual cfv entry (follow-up to 000857,
-- which flattened the topic pages under cost_of_living).
--
-- Mappings:
--   exp_food           = sum of the groceries basket (10 items)
--   exp_transportation = the monthly transit pass
--   exp_utilities      = utilities_85sqm
--   exp_internet       = internet_60
--   exp_mobile         = mobile_10gb
--   exp_entertainment  = sum of the lifestyle items
--   monthly_total      = sum of all itemized sources above
--
-- exp_accommodation, exp_health, family_total, and rent_1br stay manual —
-- no source group exists for them (housing is a separate category).
-- The merge uses options || so existing option keys are preserved.
--
-- Ref: .jeos/sections/80-destination-wiki.md

BEGIN;

UPDATE custom_fields f
SET options = COALESCE(f.options, '{}'::jsonb) || m.spec::jsonb
FROM (VALUES
	('exp_food', '{"computed":{"metric":"sum","sources":[
		{"group":"wiki_city_prices_food_groceries","key":"milk_1l"},
		{"group":"wiki_city_prices_food_groceries","key":"bread_500g"},
		{"group":"wiki_city_prices_food_groceries","key":"rice_1kg"},
		{"group":"wiki_city_prices_food_groceries","key":"eggs_12"},
		{"group":"wiki_city_prices_food_groceries","key":"chicken_1kg"},
		{"group":"wiki_city_prices_food_groceries","key":"beef_1kg"},
		{"group":"wiki_city_prices_food_groceries","key":"apples_1kg"},
		{"group":"wiki_city_prices_food_groceries","key":"bananas_1kg"},
		{"group":"wiki_city_prices_food_groceries","key":"tomatoes_1kg"},
		{"group":"wiki_city_prices_food_groceries","key":"potatoes_1kg"}]}}'),
	('exp_transportation', '{"computed":{"metric":"avg","sources":[
		{"group":"wiki_city_prices_transportation","key":"ttc_monthly_adult"}]}}'),
	('exp_utilities', '{"computed":{"metric":"avg","sources":[
		{"group":"wiki_city_prices_utilities","key":"utilities_85sqm"}]}}'),
	('exp_internet', '{"computed":{"metric":"avg","sources":[
		{"group":"wiki_city_prices_utilities","key":"internet_60"}]}}'),
	('exp_mobile', '{"computed":{"metric":"avg","sources":[
		{"group":"wiki_city_prices_utilities","key":"mobile_10gb"}]}}'),
	('exp_entertainment', '{"computed":{"metric":"sum","sources":[
		{"group":"wiki_city_prices_lifestyle","key":"gym_monthly"},
		{"group":"wiki_city_prices_lifestyle","key":"movie_ticket"},
		{"group":"wiki_city_prices_lifestyle","key":"netflix_standard"}]}}'),
	('monthly_total', '{"computed":{"metric":"sum","sources":[
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
		{"group":"wiki_city_prices_lifestyle","key":"netflix_standard"}]}}')
) AS m(field_key, spec)
JOIN custom_field_groups g ON g.slug = 'wiki_city_cost_of_living'
WHERE f.group_id = g.id
  AND f.key = m.field_key;

-- The summary row leads the pages rollup: the category's own group
-- (its computed exp_*/monthly_total fields) renders as the first row
-- ahead of the five topic rows.
UPDATE wiki_type_categories c
SET rollup = c.rollup || '{"include_self":true}'::jsonb
FROM space_types st
WHERE c.space_type_id = st.id
  AND st.slug = 'city'
  AND c.category = 'cost_of_living'
  AND c.rollup ? 'mode'
  AND c.rollup->>'mode' = 'pages';

COMMIT;
