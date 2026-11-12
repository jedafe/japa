-- New comparison types for space types that had wiki categories but no
-- comparable type: regions (location), programs (education), attractions
-- (location). Mirrors 001072 — bound overview group + full category pool.

BEGIN;

INSERT INTO comparison_type_definitions
    (name, slug, description, icon, field_group_id, item_label, is_system, config)
SELECT 'Regions', 'regions', 'Compare regions across key dimensions', 'globe',
       g.id, 'Region', true,
       '{"level":"region","family":"location","item_ref_types":["region","comparison"],"max_item_count":5,"default_item_count":3,"allowed_nested_types":["countries","states","cities"],"allow_nested_comparisons":true}'::jsonb
FROM custom_field_groups g WHERE g.slug = 'wiki_region_overview'
ON CONFLICT (slug) DO NOTHING;

INSERT INTO comparison_type_definitions
    (name, slug, description, icon, field_group_id, item_label, is_system, config)
SELECT 'Programs', 'programs', 'Compare academic programs across key dimensions', 'graduation-cap',
       g.id, 'Program', true,
       '{"level":"program","family":"education","item_ref_types":["program","comparison"],"max_item_count":5,"default_item_count":3,"allowed_nested_types":["universities","courses"],"allow_nested_comparisons":true}'::jsonb
FROM custom_field_groups g WHERE g.slug = 'wiki_program_overview'
ON CONFLICT (slug) DO NOTHING;

INSERT INTO comparison_type_definitions
    (name, slug, description, icon, field_group_id, item_label, is_system, config)
SELECT 'Attractions', 'attractions', 'Compare attractions across key dimensions', 'map-pin',
       g.id, 'Attraction', true,
       '{"level":"attraction","family":"location","item_ref_types":["attraction","comparison"],"max_item_count":5,"default_item_count":3,"allowed_nested_types":["cities","countries"],"allow_nested_comparisons":true}'::jsonb
FROM custom_field_groups g WHERE g.slug = 'wiki_attraction_overview'
ON CONFLICT (slug) DO NOTHING;

-- Pools: every category group of the mapped space type; is_default marks
-- the bound overview group (same shape as 001072's wiki-bound seeding).
INSERT INTO comparison_type_field_groups (type_id, group_id, category, label, position, is_default)
SELECT td.id, wtc.field_group_id, wtc.slug, wtc.label, wtc.position,
       (wtc.field_group_id = td.field_group_id)
FROM comparison_type_definitions td
JOIN (VALUES
    ('regions',    'region'),
    ('programs',   'program'),
    ('attractions','attraction')
) AS m(type_slug, space_slug) ON m.type_slug = td.slug
JOIN space_types st ON st.slug = m.space_slug
JOIN wiki_type_categories wtc ON wtc.space_type_id = st.id
ON CONFLICT DO NOTHING;

COMMIT;
