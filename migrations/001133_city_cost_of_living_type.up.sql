-- City-scoped cost-of-living rollup type — the missing sibling of
-- country-cost-of-living / state-cost-of-living on the cost-of-living
-- family page. Binds the existing wiki_city_cost_of_living group (15
-- rollup fields) exactly like the country/state types bind theirs.

BEGIN;

INSERT INTO comparison_type_definitions
    (name, slug, description, icon, field_group_id, item_label, is_system, config)
SELECT 'City Cost of Living', 'city-cost-of-living',
       'Compare cost-of-living rollups between cities', NULL,
       g.id, 'City', true,
       '{"level":"city","family":"cost-of-living","item_ref_types":["city","comparison"],"max_item_count":5,"default_item_count":3,"allowed_nested_types":["universities","cost-of-living","banks","courses","visa-types","mentors","countries","states","cities"],"allow_nested_comparisons":true}'::jsonb
FROM custom_field_groups g WHERE g.slug = 'wiki_city_cost_of_living'
ON CONFLICT (slug) DO NOTHING;

-- Pool: single bound group, is_default at position -1 — same shape as
-- the country/state COL types.
INSERT INTO comparison_type_field_groups (type_id, group_id, category, label, position, is_default)
SELECT td.id, g.id, NULL, g.name, -1, true
FROM comparison_type_definitions td
JOIN custom_field_groups g ON g.slug = 'wiki_city_cost_of_living'
WHERE td.slug = 'city-cost-of-living'
ON CONFLICT DO NOTHING;

COMMIT;
