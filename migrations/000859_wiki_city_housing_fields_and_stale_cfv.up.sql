-- 000859_wiki_city_housing_fields_and_stale_cfv.up.sql
-- §80 page rollup amendment, follow-up: give the city housing category
-- group the canonical rent/purchase fields, relocate the stored
-- cost-of-living rent_1br values into it, wire the remaining summary
-- fields (exp_accommodation, rent_1br, monthly_total), and delete the
-- cfv rows that computed fields made unreachable.
--
-- rent_1br's stored price_range moves to wiki_city_housing.rent_1br —
-- no value is lost; the cost-of-living field re-derives it as a
-- point estimate ({avg}) via options.computed. The remaining computed
-- fields' stored rows are superseded by the fold and deleted here —
-- they are unreachable by contract (computed fields never fall back to
-- stored cfv).
--
-- Ref: .jeos/sections/80-destination-wiki.md

BEGIN;

-- 1. Canonical housing fields on the (previously empty) bound group.
INSERT INTO custom_fields (group_id, key, label, field_type, options, position, is_system, show_in_frontend)
SELECT g.id, m.key, m.label, 'price_range', '{"currency_code":"CAD"}'::jsonb, m.position, true, true
FROM custom_field_groups g
JOIN (VALUES
	('rent_1br',         'Rent — 1BR apartment (monthly)',        1),
	('rent_3br',         'Rent — 3BR apartment (monthly)',        2),
	('buy_sqm_centre',   'Buy — price per m², city centre',       3),
	('buy_sqm_outside',  'Buy — price per m², outside centre',    4)
) AS m(key, label, position) ON true
WHERE g.slug = 'wiki_city_housing'
ON CONFLICT (group_id, key) DO NOTHING;

-- 2. Field permissions matching the topic-group convention:
--    guest can_view, members can_view+can_edit.
INSERT INTO custom_field_permissions (field_id, user_group_id, can_view, can_edit)
SELECT f.id, ug.id, true, ug.slug = 'members'
FROM custom_fields f
JOIN custom_field_groups g ON g.id = f.group_id AND g.slug = 'wiki_city_housing'
JOIN user_groups ug ON ug.slug IN ('guest', 'members')
WHERE f.key IN ('rent_1br', 'rent_3br', 'buy_sqm_centre', 'buy_sqm_outside')
ON CONFLICT (field_id, user_group_id) DO NOTHING;

-- 3. Relocate stored rent_1br values to the canonical housing field —
--    each space's price_range JSON carries over verbatim.
INSERT INTO custom_field_values (field_id, entity_type, entity_id, value_json, created_at, updated_at)
SELECT hf.id, v.entity_type, v.entity_id, v.value_json, now(), now()
FROM custom_field_values v
JOIN custom_fields sf ON sf.id = v.field_id AND sf.key = 'rent_1br'
JOIN custom_field_groups sg ON sg.id = sf.group_id AND sg.slug = 'wiki_city_cost_of_living'
JOIN custom_fields hf ON hf.key = 'rent_1br'
JOIN custom_field_groups hg ON hg.id = hf.group_id AND hg.slug = 'wiki_city_housing'
WHERE v.entity_type = 'space'
  AND v.value_json IS NOT NULL
ON CONFLICT DO NOTHING;

-- 4. Wire the remaining cost-of-living summary fields:
--    rent_1br / exp_accommodation fold the canonical housing rent;
--    monthly_total gains the rent source (single person = 1BR).
UPDATE custom_fields f
SET options = COALESCE(f.options, '{}'::jsonb) || m.spec::jsonb
FROM (VALUES
	('rent_1br', '{"computed":{"metric":"avg","sources":[
		{"group":"wiki_city_housing","key":"rent_1br"}]}}'),
	('exp_accommodation', '{"computed":{"metric":"avg","sources":[
		{"group":"wiki_city_housing","key":"rent_1br"}]}}'),
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
		{"group":"wiki_city_prices_lifestyle","key":"netflix_standard"},
		{"group":"wiki_city_housing","key":"rent_1br"}]}}')
) AS m(field_key, spec)
JOIN custom_field_groups g ON g.slug = 'wiki_city_cost_of_living'
WHERE f.group_id = g.id
  AND f.key = m.field_key;

-- 5. Drop stored cfv on every computed field — unreachable by contract.
DELETE FROM custom_field_values v
USING custom_fields f
WHERE v.field_id = f.id
  AND f.options ? 'computed';

-- 6. JCE hydration requires item_ref on the comparison item — without
--    it the matrix never calls the resolver and cells render empty.
--    cost_of_living items self-reference as col_city so their matrix
--    cells hydrate from the geo space's cfv (the seam that now carries
--    computed values). Persisted comparison_values still win per cell —
--    hydration only fills gaps.
UPDATE comparison_items i
SET item_ref = jsonb_build_object('type', 'col_city', 'id', i.id::text)
FROM comparison_type_definitions t
WHERE i.type_id = t.id
  AND t.slug = 'cost_of_living'
  AND i.deleted_at IS NULL
  AND (i.item_ref IS NULL OR i.item_ref = '{}'::jsonb);

COMMIT;
