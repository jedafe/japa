-- db/seed/wiki_example_cost_of_living.sql
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md
-- (Wiki Topic Pages, Itemized Pricing, Hierarchical UX)
--
-- Demo dataset portraying the cost_of_living wiki mockups end-to-end:
--   Canada  → Cost of Living   (country rollup over cities)
--   Toronto → Cost of Living   (category page + quick facts)
--   Toronto → Prices           (parent page — composite previews of the
--                               five child topic groups)
--   Toronto → Transportation Prices (topic page — price_parts tables:
--                               TTC, Taxis & Rideshare, Car Ownership,
--                               Biking & Micromobility)
--
-- Idempotent: safe to run repeatedly (upserts on natural keys).
--   psql "$JC_DATABASE_URL" -f db/seed/wiki_example_cost_of_living.sql

BEGIN;

-- ─────────────────────────────────────────────────────────────────────
-- 1. Demo author (byline "By amaka_okoro" on the seeded pages).
-- ─────────────────────────────────────────────────────────────────────

INSERT INTO users (id, email, username, is_synthetic, synthetic_source, status)
VALUES ('f1e2d3c4-b5a6-4789-9abc-def012345678',
        'amaka.okoro@japa.to', 'amaka_okoro', true, 'wiki_demo', 'active')
ON CONFLICT (email) DO NOTHING;

INSERT INTO user_profiles (user_id, display_name, bio)
SELECT u.id, 'Amaka Okoro', 'Community editor — Toronto transit & cost of living.'
FROM users u WHERE u.username = 'amaka_okoro'
ON CONFLICT (user_id) DO UPDATE SET display_name = EXCLUDED.display_name;

-- ─────────────────────────────────────────────────────────────────────
-- 2. Field definitions.
-- ─────────────────────────────────────────────────────────────────────

-- City cost-of-living group: image "Cost of Living in Toronto" — headline
-- ranges (drive the country rollup), the monthly-expense breakdown, and
-- the topic-scoped Quick Facts rail.
UPDATE custom_fields SET field_type = 'price_range',
       label = 'Single person (monthly)', section = 'summary',
       options = NULL
WHERE key = 'monthly_total'
  AND group_id = (SELECT id FROM custom_field_groups WHERE slug = 'wiki_city_cost_of_living');

UPDATE custom_fields SET label = 'Rent — 1BR city centre (monthly)',
       options = NULL
WHERE key = 'rent_1br'
  AND group_id = (SELECT id FROM custom_field_groups WHERE slug = 'wiki_city_cost_of_living');

WITH g AS (SELECT id FROM custom_field_groups WHERE slug = 'wiki_city_cost_of_living')
INSERT INTO custom_fields (group_id, key, label, field_type, section, position, options)
SELECT g.id, v.key, v.label, v.ftype, v.sec, v.pos, v.opts::jsonb
FROM g, (VALUES
	('family_total',        'Family of 4 (monthly)',      'price_range', 'summary',          2,  NULL),
	('exp_accommodation',   'Accommodation',              'currency',    'monthly_expenses', 10, NULL),
	('exp_food',            'Food',                       'currency',    'monthly_expenses', 11, NULL),
	('exp_transportation',  'Transportation',             'currency',    'monthly_expenses', 12, NULL),
	('exp_utilities',       'Utilities',                  'currency',    'monthly_expenses', 13, NULL),
	('exp_internet',        'Internet',                   'currency',    'monthly_expenses', 14, NULL),
	('exp_mobile',          'Mobile plan',                'currency',    'monthly_expenses', 15, NULL),
	('exp_entertainment',   'Entertainment',              'currency',    'monthly_expenses', 16, NULL),
	('exp_health',          'Health insurance',           'currency',    'monthly_expenses', 17, NULL),
	('qf_currency',         'Currency',                   'text',        'quick_facts',      20, '{"default_source":"space_currency"}'),
	('qf_monthly_cost',     'Average monthly cost',       'currency',    'quick_facts',      21, NULL),
	('qf_population',       'Population (2024)',          'text',        'quick_facts',      22, NULL),
	('qf_best_for',         'Best for',                   'text',        'quick_facts',      23, NULL)
) AS v(key, label, ftype, sec, pos, opts)
ON CONFLICT (group_id, key) DO UPDATE
SET label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    section = EXCLUDED.section, position = EXCLUDED.position,
    options = EXCLUDED.options;

-- Country cost-of-living group: the country's own monthly-expense ranges.
WITH g AS (SELECT id FROM custom_field_groups WHERE slug = 'wiki_country_cost_of_living')
INSERT INTO custom_fields (group_id, key, label, field_type, section, position, options)
SELECT g.id, v.key, v.label, v.ftype, v.sec, v.pos, v.opts::jsonb
FROM g, (VALUES
	('exp_accommodation',   'Accommodation',      'price_range', 'monthly_expenses', 1, NULL),
	('exp_food',            'Food',               'price_range', 'monthly_expenses', 2, NULL),
	('exp_transportation',  'Transportation',     'price_range', 'monthly_expenses', 3, NULL),
	('exp_utilities',       'Utilities',          'price_range', 'monthly_expenses', 4, NULL),
	('exp_internet',        'Internet',           'price_range', 'monthly_expenses', 5, NULL),
	('exp_mobile',          'Mobile plan',        'price_range', 'monthly_expenses', 6, NULL),
	('exp_entertainment',   'Entertainment',      'price_range', 'monthly_expenses', 7, NULL),
	('exp_health',          'Health insurance',   'price_range', 'monthly_expenses', 8, NULL),
	('qf_currency',         'Currency',           'text',        'quick_facts',      20, '{"default_source":"space_currency"}'),
	('qf_monthly_range',    'Average monthly cost', 'text',      'quick_facts',      21, NULL),
	('qf_best_for',         'Best for',           'text',        'quick_facts',      22, NULL)
) AS v(key, label, ftype, sec, pos, opts)
ON CONFLICT (group_id, key) DO UPDATE
SET label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    section = EXCLUDED.section, position = EXCLUDED.position,
    options = EXCLUDED.options;

-- Parent prices group: keep only the Quick Facts rail fields — the
-- previews now come from the child topic groups (no value duplication,
-- §80 Wiki Topic Pages). The flat seed fields are dropped at the end of
-- §2, once their comparison dimensions have been re-pointed.
WITH g AS (SELECT id FROM custom_field_groups WHERE slug = 'wiki_city_prices')
INSERT INTO custom_fields (group_id, key, label, field_type, section, position, options)
SELECT g.id, v.key, v.label, v.ftype, v.sec, v.pos, v.opts::jsonb
FROM g, (VALUES
	('qf_currency',      'Currency',              'text',     'quick_facts', 20, '{"default_source":"space_currency"}'),
	('qf_monthly_cost',  'Average monthly cost',  'currency', 'quick_facts', 21, NULL),
	('qf_population',    'Population (2024)',     'text',     'quick_facts', 22, NULL),
	('qf_best_for',      'Best for',              'text',     'quick_facts', 23, NULL)
) AS v(key, label, ftype, sec, pos, opts)
ON CONFLICT (group_id, key) DO UPDATE
SET label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    section = EXCLUDED.section, position = EXCLUDED.position,
    options = EXCLUDED.options;

-- Food & Groceries topic group (image: Item | Average Price | Range).
WITH g AS (SELECT id FROM custom_field_groups WHERE slug = 'wiki_city_prices_food_groceries')
INSERT INTO custom_fields (group_id, key, label, field_type, section, position, options)
SELECT g.id, v.key, v.label, 'price_range', NULL, v.pos, NULL
FROM g, (VALUES
	('milk_1l',       'Milk (1L)',                1),
	('bread_500g',    'Bread (500g)',             2),
	('rice_1kg',      'Rice (1kg)',               3),
	('eggs_12',       'Eggs (12)',                4),
	('chicken_1kg',   'Chicken fillets (1kg)',    5),
	('beef_1kg',      'Beef (1kg)',               6),
	('apples_1kg',    'Apples (1kg)',             7),
	('bananas_1kg',   'Bananas (1kg)',            8),
	('tomatoes_1kg',  'Tomatoes (1kg)',           9),
	('potatoes_1kg',  'Potatoes (1kg)',           10)
) AS v(key, label, pos)
ON CONFLICT (group_id, key) DO UPDATE
SET label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    section = EXCLUDED.section, position = EXCLUDED.position,
    options = EXCLUDED.options;

-- Restaurants topic group.
WITH g AS (SELECT id FROM custom_field_groups WHERE slug = 'wiki_city_prices_restaurants')
INSERT INTO custom_fields (group_id, key, label, field_type, section, position, options)
SELECT g.id, v.key, v.label, 'price_range', NULL, v.pos, NULL
FROM g, (VALUES
	('meal_inexpensive',   'Inexpensive meal',            1),
	('meal_two_midrange',  'Mid-range meal for two',      2),
	('mcmeal',             'McMeal (or similar combo)',   3),
	('cappuccino',         'Cappuccino (regular)',        4),
	('beer_05l',           'Beer (0.5L, restaurant)',     5),
	('wine_midrange',      'Bottle of wine (mid-range)',  6)
) AS v(key, label, pos)
ON CONFLICT (group_id, key) DO UPDATE
SET label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    section = EXCLUDED.section, position = EXCLUDED.position,
    options = EXCLUDED.options;

-- Utilities topic group.
WITH g AS (SELECT id FROM custom_field_groups WHERE slug = 'wiki_city_prices_utilities')
INSERT INTO custom_fields (group_id, key, label, field_type, section, position, options)
SELECT g.id, v.key, v.label, 'price_range', NULL, v.pos, NULL
FROM g, (VALUES
	('utilities_85sqm',  'Basic utilities (85m² apartment)', 1),
	('internet_60',      'Internet (60 Mbps+)',              2),
	('mobile_10gb',      'Mobile plan (10GB+)',              3)
) AS v(key, label, pos)
ON CONFLICT (group_id, key) DO UPDATE
SET label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    section = EXCLUDED.section, position = EXCLUDED.position,
    options = EXCLUDED.options;

-- Lifestyle topic group.
WITH g AS (SELECT id FROM custom_field_groups WHERE slug = 'wiki_city_prices_lifestyle')
INSERT INTO custom_fields (group_id, key, label, field_type, section, position, options)
SELECT g.id, v.key, v.label, 'price_range', NULL, v.pos, NULL
FROM g, (VALUES
	('gym_monthly',      'Gym membership (1 month)', 1),
	('movie_ticket',     'Movie ticket',             2),
	('netflix_standard', 'Netflix (Standard plan)',  3)
) AS v(key, label, pos)
ON CONFLICT (group_id, key) DO UPDATE
SET label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    section = EXCLUDED.section, position = EXCLUDED.position,
    options = EXCLUDED.options;

-- Transportation topic group — the image-6 page. price_parts fields:
-- the TTC/Biking tables are Item | Price | Details ({price, details});
-- Taxis is Service | Base Fare | Price per km | Price per min | Minimum;
-- Car Ownership is Expense | Min | Max | Details with an emphasized
-- total row.
WITH g AS (SELECT id FROM custom_field_groups WHERE slug = 'wiki_city_prices_transportation')
INSERT INTO custom_fields (group_id, key, label, field_type, section, position, options)
SELECT g.id, v.key, v.label, 'price_parts', v.sec, v.pos, v.opts::jsonb
FROM g, (VALUES
	-- Public Transportation (TTC)
	('ttc_oneway_adult',    'One-way Ticket (Adult)',   'ttc', 1,
		'{"parts":[{"key":"price","label":"Price","kind":"money"},{"key":"details","label":"Details","kind":"text"}],"primary_part":"price"}'),
	('ttc_day_pass',        'One-day Pass',             'ttc', 2,
		'{"parts":[{"key":"price","label":"Price","kind":"money"},{"key":"details","label":"Details","kind":"text"}],"primary_part":"price"}'),
	('ttc_weekly_pass',     'Weekly Pass',              'ttc', 3,
		'{"parts":[{"key":"price","label":"Price","kind":"money"},{"key":"details","label":"Details","kind":"text"}],"primary_part":"price"}'),
	('ttc_monthly_adult',   'Monthly Pass (Adult)',     'ttc', 4,
		'{"parts":[{"key":"price","label":"Price","kind":"money"},{"key":"details","label":"Details","kind":"text"}],"primary_part":"price"}'),
	('ttc_monthly_student', 'Monthly Pass (Student)',   'ttc', 5,
		'{"parts":[{"key":"price","label":"Price","kind":"money"},{"key":"details","label":"Details","kind":"text"}],"primary_part":"price"}'),
	('ttc_monthly_senior',  'Monthly Pass (Senior)',    'ttc', 6,
		'{"parts":[{"key":"price","label":"Price","kind":"money"},{"key":"details","label":"Details","kind":"text"}],"primary_part":"price"}'),
	('ttc_presto_card',     'PRESTO Card',              'ttc', 7,
		'{"parts":[{"key":"price","label":"Price","kind":"money"},{"key":"details","label":"Details","kind":"text"}],"primary_part":"price"}'),
	-- Taxis & Rideshare
	('taxi_traditional',    'Taxi (Traditional)',       'taxis', 10,
		'{"parts":[{"key":"base_fare","label":"Base Fare","kind":"money"},{"key":"per_km","label":"Price per km","kind":"money"},{"key":"per_min","label":"Price per min","kind":"money"},{"key":"minimum","label":"Minimum Fare","kind":"money"}],"primary_part":"per_km"}'),
	('taxi_uberx',          'UberX',                    'taxis', 11,
		'{"parts":[{"key":"base_fare","label":"Base Fare","kind":"money"},{"key":"per_km","label":"Price per km","kind":"money"},{"key":"per_min","label":"Price per min","kind":"money"},{"key":"minimum","label":"Minimum Fare","kind":"money"}],"primary_part":"per_km"}'),
	('taxi_uberxl',         'UberXL',                   'taxis', 12,
		'{"parts":[{"key":"base_fare","label":"Base Fare","kind":"money"},{"key":"per_km","label":"Price per km","kind":"money"},{"key":"per_min","label":"Price per min","kind":"money"},{"key":"minimum","label":"Minimum Fare","kind":"money"}],"primary_part":"per_km"}'),
	('taxi_lyft',           'Lyft',                     'taxis', 13,
		'{"parts":[{"key":"base_fare","label":"Base Fare","kind":"money"},{"key":"per_km","label":"Price per km","kind":"money"},{"key":"per_min","label":"Price per min","kind":"money"},{"key":"minimum","label":"Minimum Fare","kind":"money"}],"primary_part":"per_km"}'),
	-- Car Ownership (Monthly Costs) — the total row is emphasized.
	('car_loan',            'Car Loan Payment',         'car_ownership', 20,
		'{"parts":[{"key":"monthly_min","label":"Monthly (min)","kind":"money"},{"key":"monthly_max","label":"Monthly (max)","kind":"money"},{"key":"details","label":"Details","kind":"text"}],"primary_part":"monthly_min"}'),
	('car_insurance',       'Insurance',                'car_ownership', 21,
		'{"parts":[{"key":"monthly_min","label":"Monthly (min)","kind":"money"},{"key":"monthly_max","label":"Monthly (max)","kind":"money"},{"key":"details","label":"Details","kind":"text"}],"primary_part":"monthly_min"}'),
	('car_fuel',            'Fuel',                     'car_ownership', 22,
		'{"parts":[{"key":"monthly_min","label":"Monthly (min)","kind":"money"},{"key":"monthly_max","label":"Monthly (max)","kind":"money"},{"key":"details","label":"Details","kind":"text"}],"primary_part":"monthly_min"}'),
	('car_parking',         'Parking',                  'car_ownership', 23,
		'{"parts":[{"key":"monthly_min","label":"Monthly (min)","kind":"money"},{"key":"monthly_max","label":"Monthly (max)","kind":"money"},{"key":"details","label":"Details","kind":"text"}],"primary_part":"monthly_min"}'),
	('car_maintenance',     'Maintenance & Repairs',    'car_ownership', 24,
		'{"parts":[{"key":"monthly_min","label":"Monthly (min)","kind":"money"},{"key":"monthly_max","label":"Monthly (max)","kind":"money"},{"key":"details","label":"Details","kind":"text"}],"primary_part":"monthly_min"}'),
	('car_other',           'Other Costs (wash, etc.)', 'car_ownership', 25,
		'{"parts":[{"key":"monthly_min","label":"Monthly (min)","kind":"money"},{"key":"monthly_max","label":"Monthly (max)","kind":"money"},{"key":"details","label":"Details","kind":"text"}],"primary_part":"monthly_min"}'),
	('car_total',           'Total Estimated Monthly Cost', 'car_ownership', 26,
		'{"parts":[{"key":"monthly_min","label":"Monthly (min)","kind":"money"},{"key":"monthly_max","label":"Monthly (max)","kind":"money"},{"key":"details","label":"Details","kind":"text"}],"primary_part":"monthly_min","emphasize":true}'),
	-- Biking & Micromobility
	('bike_single',         'Bike Share Toronto (Single Ride)',    'biking', 30,
		'{"parts":[{"key":"price","label":"Price","kind":"money"},{"key":"details","label":"Details","kind":"text"}],"primary_part":"price"}'),
	('bike_day_pass',       'Bike Share Toronto (24-Hour Pass)',   'biking', 31,
		'{"parts":[{"key":"price","label":"Price","kind":"money"},{"key":"details","label":"Details","kind":"text"}],"primary_part":"price"}'),
	('escooter',            'Electric Scooter (Bird/Lime)',        'biking', 32,
		'{"parts":[{"key":"price","label":"Price","kind":"money"},{"key":"details","label":"Details","kind":"text"}],"primary_part":"price"}'),
	('ebike_monthly',       'Monthly E-bike Rental',               'biking', 33,
		'{"parts":[{"key":"price","label":"Price","kind":"money"},{"key":"details","label":"Details","kind":"text"}],"primary_part":"price"}')
) AS v(key, label, sec, pos, opts)
ON CONFLICT (group_id, key) DO UPDATE
SET label = EXCLUDED.label, field_type = 'price_parts',
    section = EXCLUDED.section, position = EXCLUDED.position,
    options = EXCLUDED.options;

-- Transportation quick facts — text/currency, not price_parts.
WITH g AS (SELECT id FROM custom_field_groups WHERE slug = 'wiki_city_prices_transportation')
INSERT INTO custom_fields (group_id, key, label, field_type, section, position, options)
SELECT g.id, v.key, v.label, v.ftype, 'quick_facts', v.pos, v.opts::jsonb
FROM g, (VALUES
	('qf_currency',        'Currency',                    'text',     40, '{"default_source":"space_currency"}'),
	('qf_transit_monthly', 'Typical monthly transit cost','currency', 41, NULL),
	('qf_popular_transit', 'Most popular transit',        'text',     42, NULL),
	('qf_best_for',        'Best for',                    'text',     43, NULL)
) AS v(key, label, ftype, pos, opts)
ON CONFLICT (group_id, key) DO UPDATE
SET label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    section = EXCLUDED.section, position = EXCLUDED.position,
    options = EXCLUDED.options;

-- Now that the child-group fields exist: re-point the comparison
-- dimensions off the flat parent fields (monthly transit pass projects
-- the TTC adult pass's primary part; milk and cinema land on
-- price_range fields), then drop the stale fields.
UPDATE comparison_dimensions d SET source_field_id = nf.id
FROM custom_fields nf, custom_field_groups ng,
     custom_fields of2, custom_field_groups og
WHERE d.source_field_id = of2.id
  AND of2.group_id = og.id AND og.slug = 'wiki_city_prices'
  AND nf.group_id = ng.id
  AND ng.slug = CASE of2.key
		WHEN 'monthly_pass'  THEN 'wiki_city_prices_transportation'
		WHEN 'milk_1l'       THEN 'wiki_city_prices_food_groceries'
		WHEN 'cinema_ticket' THEN 'wiki_city_prices_lifestyle' END
  AND nf.key = CASE of2.key
		WHEN 'monthly_pass'  THEN 'ttc_monthly_adult'
		WHEN 'milk_1l'       THEN 'milk_1l'
		WHEN 'cinema_ticket' THEN 'movie_ticket' END;

DELETE FROM custom_fields
WHERE group_id = (SELECT id FROM custom_field_groups WHERE slug = 'wiki_city_prices')
  AND key IN ('monthly_pass', 'milk_1l', 'cinema_ticket');

-- ─────────────────────────────────────────────────────────────────────
-- 3. Field permissions — guests can view, members can view + propose
--    edits (§06 field-level RBAC).
-- ─────────────────────────────────────────────────────────────────────

INSERT INTO custom_field_permissions (field_id, user_group_id, can_view, can_edit)
SELECT f.id, '00000000-0000-0000-0000-000000000030', true, false
FROM custom_fields f
JOIN custom_field_groups g ON g.id = f.group_id
WHERE g.slug IN (
	'wiki_city_cost_of_living', 'wiki_country_cost_of_living', 'wiki_city_prices',
	'wiki_city_prices_food_groceries', 'wiki_city_prices_restaurants',
	'wiki_city_prices_transportation', 'wiki_city_prices_utilities',
	'wiki_city_prices_lifestyle'
)
ON CONFLICT (field_id, user_group_id) DO UPDATE
SET can_view = EXCLUDED.can_view;

INSERT INTO custom_field_permissions (field_id, user_group_id, can_view, can_edit)
SELECT f.id, '00000000-0000-0000-0000-000000000020', true, true
FROM custom_fields f
JOIN custom_field_groups g ON g.id = f.group_id
WHERE g.slug IN (
	'wiki_city_cost_of_living', 'wiki_country_cost_of_living', 'wiki_city_prices',
	'wiki_city_prices_food_groceries', 'wiki_city_prices_restaurants',
	'wiki_city_prices_transportation', 'wiki_city_prices_utilities',
	'wiki_city_prices_lifestyle'
)
ON CONFLICT (field_id, user_group_id) DO UPDATE
SET can_view = EXCLUDED.can_view, can_edit = EXCLUDED.can_edit;

-- ─────────────────────────────────────────────────────────────────────
-- 4. Values — entity_type 'space', keyed to the space rows.
-- ─────────────────────────────────────────────────────────────────────

-- Toronto: cost-of-living group (headline ranges + expenses + quick facts).
WITH sp AS (SELECT id FROM spaces WHERE slug = 'toronto' AND geo_code = 'CA'),
f AS (SELECT f.id, f.key, f.field_type FROM custom_fields f
      JOIN custom_field_groups g ON g.id = f.group_id
      WHERE g.slug = 'wiki_city_cost_of_living')
INSERT INTO custom_field_values (field_id, entity_type, entity_id, value_json, value_number, value_text)
SELECT f.id, 'space', sp.id, v.jsn::jsonb, v.num, v.txt
FROM sp, f
JOIN (VALUES
	('monthly_total',      '{"min":2500,"avg":2950,"max":3800}', NULL::numeric, NULL::text),
	('family_total',       '{"min":6500,"avg":7750,"max":9000}', NULL, NULL),
	('rent_1br',           '{"min":1800,"avg":2100,"max":2400}', NULL, NULL),
	('exp_accommodation',  NULL, 2100, NULL),
	('exp_food',           NULL, 450,  NULL),
	('exp_transportation', NULL, 156,  NULL),
	('exp_utilities',      NULL, 150,  NULL),
	('exp_internet',       NULL, 69,   NULL),
	('exp_mobile',         NULL, 60,   NULL),
	('exp_entertainment',  NULL, 120,  NULL),
	('exp_health',         NULL, 75,   NULL),
	('qf_currency',        NULL, NULL, 'CAD'),
	('qf_monthly_cost',    NULL, 2950, NULL),
	('qf_population',      NULL, NULL, '2.93 Million'),
	('qf_best_for',        NULL, NULL, 'Career opportunities, diverse culture, student life')
) AS v(key, jsn, num, txt) ON v.key = f.key
ON CONFLICT (field_id, entity_type, entity_id) DO UPDATE
SET value_json = EXCLUDED.value_json, value_number = EXCLUDED.value_number,
    value_text = EXCLUDED.value_text, updated_at = now();

-- Six cities' headline ranges — the country rollup table + compare rail.
WITH f AS (SELECT f.id, f.key FROM custom_fields f
           JOIN custom_field_groups g ON g.id = f.group_id
           WHERE g.slug = 'wiki_city_cost_of_living'
             AND f.key IN ('monthly_total', 'family_total')),
vals AS (
	SELECT * FROM (VALUES
		('toronto',   '{"min":2500,"avg":2950,"max":3800}', '{"min":6500,"avg":7750,"max":9000}'),
		('vancouver', '{"min":2700,"avg":3350,"max":4000}', '{"min":6800,"avg":8150,"max":9500}'),
		('montreal',  '{"min":1800,"avg":2300,"max":2800}', '{"min":4800,"avg":5700,"max":6600}'),
		('calgary',   '{"min":1900,"avg":2400,"max":2900}', '{"min":5000,"avg":6000,"max":7000}'),
		('ottawa',    '{"min":1900,"avg":2400,"max":2900}', '{"min":5000,"avg":6100,"max":7200}'),
		('halifax',   '{"min":1700,"avg":2150,"max":2600}', '{"min":4600,"avg":5600,"max":6600}')
	) AS v(slug, single, family)
)
INSERT INTO custom_field_values (field_id, entity_type, entity_id, value_json)
SELECT f.id, 'space', s.id,
	CASE f.key WHEN 'monthly_total' THEN vals.single::jsonb ELSE vals.family::jsonb END
FROM vals
JOIN spaces s ON s.slug = vals.slug AND s.geo_code = 'CA'
CROSS JOIN f
ON CONFLICT (field_id, entity_type, entity_id) DO UPDATE
SET value_json = EXCLUDED.value_json, updated_at = now();

-- Canada: country-group expense ranges + quick facts.
WITH sp AS (SELECT id FROM spaces WHERE slug = 'canada'),
f AS (SELECT f.id, f.key FROM custom_fields f
      JOIN custom_field_groups g ON g.id = f.group_id
      WHERE g.slug = 'wiki_country_cost_of_living')
INSERT INTO custom_field_values (field_id, entity_type, entity_id, value_json, value_text)
SELECT f.id, 'space', sp.id, v.jsn::jsonb, v.txt
FROM sp, f
JOIN (VALUES
	('exp_accommodation',  '{"min":700,"avg":1350,"max":2000}', NULL::text),
	('exp_food',           '{"min":300,"avg":450,"max":600}',   NULL),
	('exp_transportation', '{"min":100,"avg":128,"max":156}',   NULL),
	('exp_utilities',      '{"min":100,"avg":150,"max":200}',   NULL),
	('exp_internet',       '{"min":60,"avg":75,"max":90}',      NULL),
	('exp_mobile',         '{"min":40,"avg":62,"max":85}',      NULL),
	('exp_entertainment',  '{"min":100,"avg":150,"max":200}',   NULL),
	('exp_health',         '{"min":75,"avg":112,"max":150}',    NULL),
	('qf_currency',        NULL, 'CAD'),
	('qf_monthly_range',   NULL, 'CAD 1,475 – 3,481'),
	('qf_best_for',        NULL, 'Career opportunities, natural beauty, safety')
) AS v(key, jsn, txt) ON v.key = f.key
ON CONFLICT (field_id, entity_type, entity_id) DO UPDATE
SET value_json = EXCLUDED.value_json, value_text = EXCLUDED.value_text,
    updated_at = now();

-- Toronto: prices parent group (quick facts only).
WITH sp AS (SELECT id FROM spaces WHERE slug = 'toronto' AND geo_code = 'CA'),
f AS (SELECT f.id, f.key FROM custom_fields f
      JOIN custom_field_groups g ON g.id = f.group_id
      WHERE g.slug = 'wiki_city_prices')
INSERT INTO custom_field_values (field_id, entity_type, entity_id, value_number, value_text)
SELECT f.id, 'space', sp.id, v.num, v.txt
FROM sp, f
JOIN (VALUES
	('qf_currency',     NULL::numeric, 'CAD'),
	('qf_monthly_cost', 2950,          NULL::text),
	('qf_population',   NULL,          '2.93 Million'),
	('qf_best_for',     NULL,          'Career opportunities, diverse culture, student life')
) AS v(key, num, txt) ON v.key = f.key
ON CONFLICT (field_id, entity_type, entity_id) DO UPDATE
SET value_number = EXCLUDED.value_number, value_text = EXCLUDED.value_text,
    updated_at = now();

-- Toronto: food & groceries values.
WITH sp AS (SELECT id FROM spaces WHERE slug = 'toronto' AND geo_code = 'CA'),
f AS (SELECT f.id, f.key FROM custom_fields f
      JOIN custom_field_groups g ON g.id = f.group_id
      WHERE g.slug = 'wiki_city_prices_food_groceries')
INSERT INTO custom_field_values (field_id, entity_type, entity_id, value_json)
SELECT f.id, 'space', sp.id, v.val::jsonb
FROM sp, f
JOIN (VALUES
	('milk_1l',      '{"avg":2.29,"min":1.79,"max":2.99}'),
	('bread_500g',   '{"avg":2.65,"min":1.99,"max":3.49}'),
	('rice_1kg',     '{"avg":4.10,"min":3.29,"max":5.49}'),
	('eggs_12',      '{"avg":3.79,"min":2.99,"max":4.79}'),
	('chicken_1kg',  '{"avg":13.99,"min":11.99,"max":16.99}'),
	('beef_1kg',     '{"avg":16.99,"min":13.99,"max":21.99}'),
	('apples_1kg',   '{"avg":4.29,"min":3.29,"max":5.49}'),
	('bananas_1kg',  '{"avg":1.79,"min":1.29,"max":2.49}'),
	('tomatoes_1kg', '{"avg":3.49,"min":2.49,"max":4.49}'),
	('potatoes_1kg', '{"avg":2.29,"min":1.49,"max":2.99}')
) AS v(key, val) ON v.key = f.key
ON CONFLICT (field_id, entity_type, entity_id) DO UPDATE
SET value_json = EXCLUDED.value_json, updated_at = now();

-- Toronto: restaurant values.
WITH sp AS (SELECT id FROM spaces WHERE slug = 'toronto' AND geo_code = 'CA'),
f AS (SELECT f.id, f.key FROM custom_fields f
      JOIN custom_field_groups g ON g.id = f.group_id
      WHERE g.slug = 'wiki_city_prices_restaurants')
INSERT INTO custom_field_values (field_id, entity_type, entity_id, value_json)
SELECT f.id, 'space', sp.id, v.val::jsonb
FROM sp, f
JOIN (VALUES
	('meal_inexpensive',  '{"avg":18.00,"min":12.00,"max":25.00}'),
	('meal_two_midrange', '{"avg":75.00,"min":60.00,"max":100.00}'),
	('mcmeal',            '{"avg":12.49,"min":10.49,"max":14.49}'),
	('cappuccino',        '{"avg":4.50,"min":3.50,"max":5.50}'),
	('beer_05l',          '{"avg":8.50,"min":6.50,"max":10.50}'),
	('wine_midrange',     '{"avg":28.00,"min":20.00,"max":40.00}')
) AS v(key, val) ON v.key = f.key
ON CONFLICT (field_id, entity_type, entity_id) DO UPDATE
SET value_json = EXCLUDED.value_json, updated_at = now();

-- Toronto: utilities values.
WITH sp AS (SELECT id FROM spaces WHERE slug = 'toronto' AND geo_code = 'CA'),
f AS (SELECT f.id, f.key FROM custom_fields f
      JOIN custom_field_groups g ON g.id = f.group_id
      WHERE g.slug = 'wiki_city_prices_utilities')
INSERT INTO custom_field_values (field_id, entity_type, entity_id, value_json)
SELECT f.id, 'space', sp.id, v.val::jsonb
FROM sp, f
JOIN (VALUES
	('utilities_85sqm', '{"avg":150.00,"min":120.00,"max":180.00}'),
	('internet_60',     '{"avg":69.00,"min":55.00,"max":85.00}'),
	('mobile_10gb',     '{"avg":60.00,"min":45.00,"max":75.00}')
) AS v(key, val) ON v.key = f.key
ON CONFLICT (field_id, entity_type, entity_id) DO UPDATE
SET value_json = EXCLUDED.value_json, updated_at = now();

-- Toronto: lifestyle values.
WITH sp AS (SELECT id FROM spaces WHERE slug = 'toronto' AND geo_code = 'CA'),
f AS (SELECT f.id, f.key FROM custom_fields f
      JOIN custom_field_groups g ON g.id = f.group_id
      WHERE g.slug = 'wiki_city_prices_lifestyle')
INSERT INTO custom_field_values (field_id, entity_type, entity_id, value_json)
SELECT f.id, 'space', sp.id, v.val::jsonb
FROM sp, f
JOIN (VALUES
	('gym_monthly',      '{"avg":60.00,"min":40.00,"max":80.00}'),
	('movie_ticket',     '{"avg":16.00,"min":13.00,"max":19.00}'),
	('netflix_standard', '{"avg":16.49,"min":16.49,"max":16.49}')
) AS v(key, val) ON v.key = f.key
ON CONFLICT (field_id, entity_type, entity_id) DO UPDATE
SET value_json = EXCLUDED.value_json, updated_at = now();

-- Toronto: transportation values — {parts: {...}} canonical shape,
-- matching the mockup figures exactly.
WITH sp AS (SELECT id FROM spaces WHERE slug = 'toronto' AND geo_code = 'CA'),
f AS (SELECT f.id, f.key FROM custom_fields f
      JOIN custom_field_groups g ON g.id = f.group_id
      WHERE g.slug = 'wiki_city_prices_transportation')
INSERT INTO custom_field_values (field_id, entity_type, entity_id, value_json, value_number, value_text)
SELECT f.id, 'space', sp.id, v.jsn::jsonb, v.num, v.txt
FROM sp, f
JOIN (VALUES
	('ttc_oneway_adult',    '{"parts":{"price":3.35,"details":"Valid for 2 hours on TTC (bus, streetcar, subway)"}}', NULL::numeric, NULL::text),
	('ttc_day_pass',        '{"parts":{"price":13.50,"details":"Unlimited travel for a day"}}',                     NULL, NULL),
	('ttc_weekly_pass',     '{"parts":{"price":44.00,"details":"7 consecutive days"}}',                             NULL, NULL),
	('ttc_monthly_adult',   '{"parts":{"price":156.00,"details":"For ages 20–64"}}',                                NULL, NULL),
	('ttc_monthly_student', '{"parts":{"price":128.15,"details":"With valid post-secondary ID"}}',                  NULL, NULL),
	('ttc_monthly_senior',  '{"parts":{"price":128.15,"details":"For ages 65+"}}',                                  NULL, NULL),
	('ttc_presto_card',     '{"parts":{"price":6.00,"details":"Reloadable fare card (non-refundable)"}}',           NULL, NULL),
	('taxi_traditional',    '{"parts":{"base_fare":4.25,"per_km":1.75,"per_min":0.60,"minimum":6.00}}',             NULL, NULL),
	('taxi_uberx',          '{"parts":{"base_fare":2.50,"per_km":1.25,"per_min":0.35,"minimum":6.00}}',             NULL, NULL),
	('taxi_uberxl',         '{"parts":{"base_fare":3.50,"per_km":1.75,"per_min":0.45,"minimum":8.00}}',             NULL, NULL),
	('taxi_lyft',           '{"parts":{"base_fare":2.50,"per_km":1.20,"per_min":0.35,"minimum":6.00}}',             NULL, NULL),
	('car_loan',            '{"parts":{"monthly_min":450,"monthly_max":700,"details":"Depends on car type and loan terms"}}',      NULL, NULL),
	('car_insurance',       '{"parts":{"monthly_min":200,"monthly_max":350,"details":"Varies by age, driving history, coverage"}}', NULL, NULL),
	('car_fuel',            '{"parts":{"monthly_min":120,"monthly_max":180,"details":"Based on average 1,000 km/month"}}',          NULL, NULL),
	('car_parking',         '{"parts":{"monthly_min":80,"monthly_max":250,"details":"Street parking or lot (varies by area)"}}',    NULL, NULL),
	('car_maintenance',     '{"parts":{"monthly_min":100,"monthly_max":150,"details":"Oil changes, repairs, servicing"}}',          NULL, NULL),
	('car_other',           '{"parts":{"monthly_min":30,"monthly_max":60,"details":"Car wash, tolls, small expenses"}}',            NULL, NULL),
	('car_total',           '{"parts":{"monthly_min":980,"monthly_max":1690,"details":"Estimated all-in monthly cost"}}',           NULL, NULL),
	('bike_single',         '{"parts":{"price":3.25,"details":"30 minutes"}}',                          NULL, NULL),
	('bike_day_pass',       '{"parts":{"price":15.00,"details":"Unlimited 30-minute rides"}}',          NULL, NULL),
	('escooter',            '{"parts":{"price":1.00,"details":"+ CAD 0.39/min · varies by provider"}}', NULL, NULL),
	('ebike_monthly',       '{"parts":{"price":150.00,"details":"Varies by provider and model (up to ~CAD 200)"}}', NULL, NULL),
	('qf_currency',         NULL, NULL, 'CAD'),
	('qf_transit_monthly',  NULL, 156, NULL),
	('qf_popular_transit',  NULL, NULL, 'TTC (bus, streetcar, subway)'),
	('qf_best_for',         NULL, NULL, 'Students, commuters, everyday travel')
) AS v(key, jsn, num, txt) ON v.key = f.key
ON CONFLICT (field_id, entity_type, entity_id) DO UPDATE
SET value_json = EXCLUDED.value_json, value_number = EXCLUDED.value_number,
    value_text = EXCLUDED.value_text, updated_at = now();

-- ─────────────────────────────────────────────────────────────────────
-- 5. Materialized pages — prose + byline need a wiki_pages row (lazy
--    pages carry no content).
-- ─────────────────────────────────────────────────────────────────────

INSERT INTO wiki_pages (space_id, slug, title, category, page_template_id, field_group_id, created_by, is_active)
VALUES
	((SELECT id FROM spaces WHERE slug = 'toronto' AND geo_code = 'CA'),
	 'cost-of-living', 'Cost of Living in Toronto, Canada', 'cost_of_living',
	 NULL, NULL,
	 (SELECT id FROM users WHERE username = 'amaka_okoro'), true),
	((SELECT id FROM spaces WHERE slug = 'toronto' AND geo_code = 'CA'),
	 'prices', 'Prices in Toronto', 'cost_of_living',
	 (SELECT id FROM wiki_type_page_templates WHERE slug = 'prices'),
	 (SELECT id FROM custom_field_groups WHERE slug = 'wiki_city_prices'),
	 (SELECT id FROM users WHERE username = 'amaka_okoro'), true),
	((SELECT id FROM spaces WHERE slug = 'toronto' AND geo_code = 'CA'),
	 'transportation-prices', 'Transportation Prices in Toronto', 'cost_of_living',
	 (SELECT id FROM wiki_type_page_templates WHERE slug = 'transportation-prices'),
	 (SELECT id FROM custom_field_groups WHERE slug = 'wiki_city_prices_transportation'),
	 (SELECT id FROM users WHERE username = 'amaka_okoro'), true)
ON CONFLICT (space_id, slug) WHERE deleted_at IS NULL DO UPDATE
SET title = EXCLUDED.title,
    page_template_id = EXCLUDED.page_template_id,
    field_group_id = EXCLUDED.field_group_id,
    updated_at = now();

UPDATE wiki_pages
SET title = 'Cost of Living in Canada', updated_at = now()
WHERE space_id = (SELECT id FROM spaces WHERE slug = 'canada')
  AND slug = 'cost-of-living' AND deleted_at IS NULL;

-- ─────────────────────────────────────────────────────────────────────
-- 6. Approved revisions (TipTap JSONB) — page prose + byline.
--    Idempotent via the (wiki_page_id, content_hash) dedup index.
-- ─────────────────────────────────────────────────────────────────────

-- Toronto — Cost of Living
INSERT INTO wiki_revisions (wiki_page_id, kind, content, summary, status,
                            submitted_by, reviewed_by, reviewed_at, content_hash)
SELECT p.id, 'content', '{
	"type":"doc","content":[
		{"type":"paragraph","content":[{"type":"text","text":"Monthly expenses, prices, and practical tips to help you plan your budget in Toronto."}]},
		{"type":"paragraph","content":[{"type":"text","text":"All costs are estimated for a single person and shown in Canadian Dollars (CAD). Actual costs may vary based on lifestyle and neighborhood."}]},
		{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Tips to Save Money in Toronto"}]},
		{"type":"bulletList","content":[
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Consider living further from downtown and using public transit."}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Look for student discounts on transit, attractions, and services."}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Cook at home and shop at affordable grocery stores."}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Use monthly transit passes instead of daily tickets."}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Take advantage of free events and activities around the city."}]}]}
		]},
		{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Frequently Asked Questions"}]},
		{"type":"paragraph","content":[{"type":"text","text":"Is Toronto expensive? Toronto is one of Canada’s priciest cities — a single person should budget roughly CAD 2,950/month including rent."}]},
		{"type":"paragraph","content":[{"type":"text","text":"What salary do you need? A pre-tax income of about CAD 55,000–65,000 comfortably covers the single-person estimate above."}]}
	]
}'::jsonb, 'Initial cost-of-living guide for Toronto.', 'approved',
	(SELECT id FROM users WHERE username = 'amaka_okoro'),
	(SELECT id FROM users WHERE username = 'admin'), now(),
	encode(sha256('wiki-demo-toronto-cost-of-living-v1'::bytea), 'hex')
FROM wiki_pages p
WHERE p.space_id = (SELECT id FROM spaces WHERE slug = 'toronto' AND geo_code = 'CA')
  AND p.slug = 'cost-of-living' AND p.deleted_at IS NULL
  AND NOT EXISTS (
	SELECT 1 FROM wiki_revisions r
	WHERE r.wiki_page_id = p.id
	  AND r.content_hash = encode(sha256('wiki-demo-toronto-cost-of-living-v1'::bytea), 'hex')
  );

-- Toronto — Prices
INSERT INTO wiki_revisions (wiki_page_id, kind, content, summary, status,
                            submitted_by, reviewed_by, reviewed_at, content_hash)
SELECT p.id, 'content', '{
	"type":"doc","content":[
		{"type":"paragraph","content":[{"type":"text","text":"Detailed pricing for goods, services, and everyday expenses in Toronto, Canada."}]},
		{"type":"paragraph","content":[{"type":"text","text":"All prices are in Canadian Dollars (CAD) and are average estimates for 2024. Prices may vary by location, brand, and season."}]},
		{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Tips to Save Money"}]},
		{"type":"bulletList","content":[
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Shop at budget-friendly stores like No Frills, FreshCo, and Walmart to save on groceries."}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Bundle internet and mobile plans with providers like Rogers, Bell or Fido to get better deals."}]}]}
		]},
		{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Frequently Asked Questions"}]},
		{"type":"paragraph","content":[{"type":"text","text":"How often do these prices change? Figures are community-verified averages — suggest an edit if you spot a drift."}]}
	]
}'::jsonb, 'Initial prices overview for Toronto.', 'approved',
	(SELECT id FROM users WHERE username = 'amaka_okoro'),
	(SELECT id FROM users WHERE username = 'admin'), now(),
	encode(sha256('wiki-demo-toronto-prices-v1'::bytea), 'hex')
FROM wiki_pages p
WHERE p.space_id = (SELECT id FROM spaces WHERE slug = 'toronto' AND geo_code = 'CA')
  AND p.slug = 'prices' AND p.deleted_at IS NULL
  AND NOT EXISTS (
	SELECT 1 FROM wiki_revisions r
	WHERE r.wiki_page_id = p.id
	  AND r.content_hash = encode(sha256('wiki-demo-toronto-prices-v1'::bytea), 'hex')
  );

-- Toronto — Transportation Prices (the image-6 page).
INSERT INTO wiki_revisions (wiki_page_id, kind, content, summary, status,
                            submitted_by, reviewed_by, reviewed_at, content_hash)
SELECT p.id, 'content', '{
	"type":"doc","content":[
		{"type":"paragraph","content":[{"type":"text","text":"Detailed public transit, taxi, rideshare, and vehicle ownership costs in Toronto, Canada."}]},
		{"type":"paragraph","content":[{"type":"text","text":"All prices are in Canadian Dollars (CAD) and are average estimates for 2024. Prices may vary based on time of day, provider, and location."}]},
		{"type":"paragraph","content":[{"type":"text","text":"Toronto has an excellent public transit system operated by the Toronto Transit Commission (TTC). PRESTO offers auto-load and balance protection — you also get free transfers within 2 hours of tapping."}]},
		{"type":"paragraph","content":[{"type":"text","text":"Taxi and rideshare fares vary depending on distance, traffic, and time of day; prices are estimates and can surge during peak hours, events, or bad weather."}]},
		{"type":"paragraph","content":[{"type":"text","text":"Living near public transit can save you thousands annually on car ownership — estimated monthly costs above include loan, insurance, fuel, parking, and upkeep."}]},
		{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Tips to Save on Transportation"}]},
		{"type":"bulletList","content":[
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Use a PRESTO card for cheaper fares and transfers."}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Consider a monthly TTC pass if you use public transit regularly."}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Bike or walk for short trips to save money."}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Carpool or use rideshare with friends during peak hours."}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Live near transit lines to save on transportation costs."}]}]}
		]},
		{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Frequently Asked Questions"}]},
		{"type":"paragraph","content":[{"type":"text","text":"Is the TTC good value? A monthly adult pass (CAD 156) breaks even at roughly two round-trips per weekday."}]},
		{"type":"paragraph","content":[{"type":"text","text":"Is Uber cheaper than a taxi? On short trips the lower base fare usually wins; during surge a traditional taxi can be cheaper."}]}
	]
}'::jsonb, 'Initial transportation price guide for Toronto.', 'approved',
	(SELECT id FROM users WHERE username = 'amaka_okoro'),
	(SELECT id FROM users WHERE username = 'admin'), now(),
	encode(sha256('wiki-demo-toronto-transportation-prices-v1'::bytea), 'hex')
FROM wiki_pages p
WHERE p.space_id = (SELECT id FROM spaces WHERE slug = 'toronto' AND geo_code = 'CA')
  AND p.slug = 'transportation-prices' AND p.deleted_at IS NULL
  AND NOT EXISTS (
	SELECT 1 FROM wiki_revisions r
	WHERE r.wiki_page_id = p.id
	  AND r.content_hash = encode(sha256('wiki-demo-toronto-transportation-prices-v1'::bytea), 'hex')
  );

-- Canada — Cost of Living (row already exists from the baseline seed).
INSERT INTO wiki_revisions (wiki_page_id, kind, content, summary, status,
                            submitted_by, reviewed_by, reviewed_at, content_hash)
SELECT p.id, 'content', '{
	"type":"doc","content":[
		{"type":"paragraph","content":[{"type":"text","text":"An overview of typical monthly expenses and prices in Canada to help you plan your budget."}]},
		{"type":"paragraph","content":[{"type":"text","text":"Costs vary depending on the city, lifestyle, and personal circumstances. All amounts are in Canadian Dollars (CAD)."}]},
		{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Overview"}]},
		{"type":"paragraph","content":[{"type":"text","text":"Canada offers a high quality of life, but the cost of living can vary significantly depending on the city and your lifestyle. Major cities like Toronto and Vancouver are generally more expensive, while smaller cities and towns are more affordable."}]},
		{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Tips to Manage Your Expenses"}]},
		{"type":"bulletList","content":[
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Share accommodation to reduce rent."}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Use public transportation and student discounts."}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Cook at home and plan your meals."}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Compare mobile and internet plans for the best deals."}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Track your spending and create a monthly budget."}]}]}
		]},
		{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Frequently Asked Questions"}]},
		{"type":"paragraph","content":[{"type":"text","text":"Which city is cheapest? Of the major metros, Montreal and Halifax are typically the most affordable."}]}
	]
}'::jsonb, 'Initial cost-of-living overview for Canada.', 'approved',
	(SELECT id FROM users WHERE username = 'amaka_okoro'),
	(SELECT id FROM users WHERE username = 'admin'), now(),
	encode(sha256('wiki-demo-canada-cost-of-living-v1'::bytea), 'hex')
FROM wiki_pages p
WHERE p.space_id = (SELECT id FROM spaces WHERE slug = 'canada')
  AND p.slug = 'cost-of-living' AND p.deleted_at IS NULL
  AND NOT EXISTS (
	SELECT 1 FROM wiki_revisions r
	WHERE r.wiki_page_id = p.id
	  AND r.content_hash = encode(sha256('wiki-demo-canada-cost-of-living-v1'::bytea), 'hex')
  );

COMMIT;
