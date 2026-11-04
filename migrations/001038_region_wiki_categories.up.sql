-- 001038_region_wiki_categories.up.sql
-- §wiki-seo — give the region hub type the same wiki treatment the geo
-- types received in 001035: a real category template set instead of the
-- bare overview/countries pair. Scope is adjusted for the level (children
-- are countries): geo directories roll up entity children across the
-- region's whole subtree, and cost-of-living aggregates country metrics.
BEGIN;

-- Field groups (empty for directory categories, per the 000795 pattern).
INSERT INTO custom_field_groups (name, slug, is_system) VALUES
    ('Cost of Living (region)', 'wiki_region_cost_of_living', true),
    ('Universities (region)',   'wiki_region_universities',   true),
    ('Attractions (region)',    'wiki_region_attractions',    true),
    ('Embassies (region)',      'wiki_region_embassies',      true),
    ('Banking (region)',        'wiki_region_banking',        true),
    ('Scholarships (region)',   'wiki_region_scholarships',   true),
    ('Other (region)',          'wiki_region_other',          true)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO wiki_type_categories
    (space_type_id, slug, category, label, field_group_id, position, is_system, rollup, icon)
SELECT '00000000-0000-0000-0000-00000000007f',
       v.slug, v.category, v.label, g.id, v.position, true, v.rollup::jsonb, v.icon
FROM (VALUES
    ('cost-of-living', 'cost_of_living', 'Cost of Living',          'wiki_region_cost_of_living', 3,  '{"levels":["country","state","city"],"enabled":true,"metrics":["min","avg","max"],"section_limit":10}'::jsonb, NULL),
    ('universities',   'universities',   '{name} Universities',     'wiki_region_universities',   4,  '{"mode":"directory","levels":["university","education"],"enabled":true}'::jsonb, 'graduation-cap'),
    ('attractions',    'attractions',    '{name} Attractions',      'wiki_region_attractions',    5,  '{"mode":"directory","levels":["attraction"],"enabled":true}'::jsonb,        'landmark'),
    ('embassies',      'embassies',      '{name} Embassies',        'wiki_region_embassies',      6,  '{"mode":"directory","levels":["embassy"],"enabled":true}'::jsonb,           'building-2'),
    ('banking',        'banking',        'Banking',                 'wiki_region_banking',        7,  '{"mode":"directory","levels":["bank"],"enabled":true}'::jsonb,              NULL),
    ('scholarships',   'scholarships',   '{name} Scholarships',     'wiki_region_scholarships',   8,  '{"mode":"directory","levels":["scholarship"],"enabled":true}'::jsonb,       'award'),
    ('other',          'other',          'Other',                   'wiki_region_other',          9,  NULL,                                                                       NULL)
) AS v(slug, category, label, group_slug, position, rollup, icon)
JOIN custom_field_groups g ON g.slug = v.group_slug
ON CONFLICT (space_type_id, slug) DO NOTHING;

-- Align the seeded countries directory label with the {name} convention.
UPDATE wiki_type_categories SET label = '{name} Countries'
WHERE space_type_id = '00000000-0000-0000-0000-00000000007f' AND slug = 'countries';

-- Attach the groups to the region type's field schema.
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT g.id, 'space_type:region', NULL::uuid,
       ROW_NUMBER() OVER (ORDER BY g.slug) + 1
FROM custom_field_groups g
WHERE g.slug IN (
    'wiki_region_overview', 'wiki_region_countries', 'wiki_region_cost_of_living',
    'wiki_region_universities', 'wiki_region_attractions', 'wiki_region_embassies',
    'wiki_region_banking', 'wiki_region_scholarships', 'wiki_region_other'
)
AND NOT EXISTS (
    SELECT 1 FROM custom_field_group_assignments a
    WHERE a.group_id = g.id AND a.entity_class = 'space_type:region' AND a.entity_id IS NULL
);

COMMIT;
