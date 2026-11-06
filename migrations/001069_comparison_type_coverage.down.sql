-- 001069_comparison_type_coverage.down.sql
-- Drop the four new types and recreate the two deleted dead types
-- (approximate — original configs had empty item_ref_types).

BEGIN;

DELETE FROM comparison_type_definitions
WHERE slug IN ('embassies', 'neighborhoods', 'job-listings', 'housing');

INSERT INTO comparison_type_definitions (slug, name, description, field_group_id, config, is_system)
SELECT 'city-prices', 'City Prices',
    'Compare quick-fact price indicators across cities.',
    g.id,
    '{"level":"city","family":"geo","item_ref_types":[],"max_item_count":5,"default_item_count":3,"allow_nested_comparisons":false}'::jsonb,
    true
FROM custom_field_groups g WHERE g.slug = 'wiki_city_prices'
ON CONFLICT (slug) DO NOTHING;

INSERT INTO comparison_type_definitions (slug, name, description, field_group_id, config, is_system)
SELECT 'city-transportation-prices', 'City Transportation Prices',
    'Compare transportation prices across cities.',
    g.id,
    '{"level":"city","family":"geo","item_ref_types":[],"max_item_count":5,"default_item_count":3,"allow_nested_comparisons":false}'::jsonb,
    true
FROM custom_field_groups g WHERE g.slug = 'wiki_city_prices_transportation'
ON CONFLICT (slug) DO NOTHING;

COMMIT;
