-- 000898_jce_wiki_bound_col_types.up.sql
-- JCE ↔ Wiki field-group mapping (docs/canva/jce-to-wiki-pages-fieldgroup-mapping,
-- spec 2026-10-20-jce-wiki-field-group-mapping-design).
--
-- Walking skeleton: bind comparison types to wiki-owned field groups.
-- Cost of Living is the first entity type: three host-level types bound to
-- the per-level wiki_{level}_cost_of_living groups, plus five subtype types
-- bound to the dedicated city topic-page groups (own-group subpage model —
-- no config.sections slicing).
--
-- config.family groups types in the /compare selector; config.level names
-- the host wiki-type level; config.subtype names the wiki subpage.
--
-- Existing types get family/level tags only — their field_group bindings are
-- untouched (geo_fields → wiki_*_overview rebinding is spec open-question #3).
--
-- Ref: docs/superpowers/specs/2026-10-20-jce-wiki-field-group-mapping-design.md

BEGIN;

-- ---------------------------------------------------------------------------
-- Cost of Living — host-level types (one per level: groups are disjoint).
-- ---------------------------------------------------------------------------
INSERT INTO comparison_type_definitions (name, slug, description, field_group_id, item_label, is_system, config)
SELECT v.name, v.slug, v.description, g.id, v.item_label, true, v.config::jsonb
FROM (VALUES
    ('Country Cost of Living', 'country-cost-of-living',
     'Compare cost-of-living rollups between countries', 'Country',
     'wiki_country_cost_of_living',
     '{"item_ref_types": ["country", "comparison"], "allow_nested_comparisons": true, "allowed_nested_types": ["universities", "cost_of_living", "banks", "courses", "visa-types", "mentors", "countries", "states", "cities"], "max_item_count": 5, "default_item_count": 3, "family": "cost-of-living", "level": "country"}'),
    ('State Cost of Living', 'state-cost-of-living',
     'Compare cost-of-living rollups between states and provinces', 'State',
     'wiki_state_cost_of_living',
     '{"item_ref_types": ["state", "comparison"], "allow_nested_comparisons": true, "allowed_nested_types": ["universities", "cost_of_living", "banks", "courses", "visa-types", "mentors", "countries", "states", "cities"], "max_item_count": 5, "default_item_count": 3, "family": "cost-of-living", "level": "state"}'),
    ('City Cost of Living', 'city-cost-of-living',
     'Compare cost-of-living rollups between cities', 'City',
     'wiki_city_cost_of_living',
     '{"item_ref_types": ["city", "comparison"], "allow_nested_comparisons": true, "allowed_nested_types": ["universities", "cost_of_living", "banks", "courses", "visa-types", "mentors", "countries", "states", "cities"], "max_item_count": 5, "default_item_count": 3, "family": "cost-of-living", "level": "city"}'),
    -- Subtypes = city COL topic pages (own dedicated groups).
    ('Transportation Prices', 'col-transportation',
     'Compare transportation prices between cities', 'City',
     'wiki_city_prices_transportation',
     '{"item_ref_types": ["city", "comparison"], "allow_nested_comparisons": true, "allowed_nested_types": ["universities", "cost_of_living", "banks", "courses", "visa-types", "mentors", "countries", "states", "cities"], "max_item_count": 5, "default_item_count": 3, "family": "cost-of-living", "level": "city", "subtype": "transportation"}'),
    ('Restaurant Prices', 'col-restaurants',
     'Compare restaurant prices between cities', 'City',
     'wiki_city_prices_restaurants',
     '{"item_ref_types": ["city", "comparison"], "allow_nested_comparisons": true, "allowed_nested_types": ["universities", "cost_of_living", "banks", "courses", "visa-types", "mentors", "countries", "states", "cities"], "max_item_count": 5, "default_item_count": 3, "family": "cost-of-living", "level": "city", "subtype": "restaurants"}'),
    ('Food & Groceries Prices', 'col-food-groceries',
     'Compare food and groceries prices between cities', 'City',
     'wiki_city_prices_food_groceries',
     '{"item_ref_types": ["city", "comparison"], "allow_nested_comparisons": true, "allowed_nested_types": ["universities", "cost_of_living", "banks", "courses", "visa-types", "mentors", "countries", "states", "cities"], "max_item_count": 5, "default_item_count": 3, "family": "cost-of-living", "level": "city", "subtype": "food-groceries"}'),
    ('Utilities Prices', 'col-utilities',
     'Compare utilities prices between cities', 'City',
     'wiki_city_prices_utilities',
     '{"item_ref_types": ["city", "comparison"], "allow_nested_comparisons": true, "allowed_nested_types": ["universities", "cost_of_living", "banks", "courses", "visa-types", "mentors", "countries", "states", "cities"], "max_item_count": 5, "default_item_count": 3, "family": "cost-of-living", "level": "city", "subtype": "utilities"}'),
    ('Lifestyle Prices', 'col-lifestyle',
     'Compare lifestyle and leisure prices between cities', 'City',
     'wiki_city_prices_lifestyle',
     '{"item_ref_types": ["city", "comparison"], "allow_nested_comparisons": true, "allowed_nested_types": ["universities", "cost_of_living", "banks", "courses", "visa-types", "mentors", "countries", "states", "cities"], "max_item_count": 5, "default_item_count": 3, "family": "cost-of-living", "level": "city", "subtype": "lifestyle"}')
) AS v(name, slug, description, item_label, group_slug, config)
JOIN custom_field_groups g ON g.slug = v.group_slug
ON CONFLICT (slug) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Family/level tags on existing system types so the selector can group them.
-- jsonb || merge — additive keys only; bindings untouched.
-- ---------------------------------------------------------------------------
UPDATE comparison_type_definitions t
SET config = t.config || jsonb_build_object('family', 'location', 'level', m.level)
FROM (VALUES
    ('countries', 'country'),
    ('states',    'state'),
    ('cities',    'city')
) AS m(slug, level)
WHERE t.slug = m.slug;

UPDATE comparison_type_definitions t
SET config = t.config || jsonb_build_object('family', 'education', 'level', m.level)
FROM (VALUES
    ('universities', 'university'),
    ('courses',      'course')
) AS m(slug, level)
WHERE t.slug = m.slug;

UPDATE comparison_type_definitions
SET config = config || '{"family": "banking", "level": "bank"}'::jsonb
WHERE slug = 'banks';

UPDATE comparison_type_definitions
SET config = config || '{"family": "visa_routes", "level": "visa_route"}'::jsonb
WHERE slug = 'visa-types';

COMMIT;
