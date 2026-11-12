-- Restores the four removed types with their exact prior configs and
-- pool rows (captured before deletion).

BEGIN;

INSERT INTO comparison_type_definitions
    (name, slug, description, icon, field_group_id, item_label, is_system, config)
VALUES
    ('Attractions', 'attractions', 'Compare attractions across key dimensions', 'map-pin',
     '3ca5a3cb-5b85-4ad4-ae3b-4b3117253507', 'Attraction', true,
     '{"level": "attraction", "family": "location", "item_ref_types": ["attraction", "comparison"], "max_item_count": 5, "default_item_count": 3, "allowed_nested_types": ["cities", "countries"], "allow_nested_comparisons": true}'::jsonb),
    ('Housing & Accommodation', 'housing', 'Compare housing and accommodation guides — rent, deposits, facilities and lease terms.', NULL,
     'a9bdc1cb-c68a-473e-b3d1-b0bf79fcb0c3', 'Item', true,
     '{"level": "housing", "family": "location", "item_ref_types": ["housing_guide", "comparison"], "max_item_count": 5, "default_item_count": 3, "allowed_nested_types": ["cities", "cost-of-living"], "allow_nested_comparisons": true}'::jsonb),
    ('Neighborhoods', 'neighborhoods', 'Compare neighborhoods — safety, amenities, transport, schools and demographics.', NULL,
     'a9e6f173-2367-475b-9c0a-f060521eacb6', 'Item', true,
     '{"level": "neighborhood", "family": "location", "item_ref_types": ["neighborhood", "comparison"], "max_item_count": 5, "default_item_count": 3, "allowed_nested_types": ["cities", "cost-of-living"], "allow_nested_comparisons": true}'::jsonb),
    ('Regions', 'regions', 'Compare regions across key dimensions', 'globe',
     '7ba78bcf-1a69-4f69-bb12-53d62c6fd677', 'Region', true,
     '{"level": "region", "family": "location", "item_ref_types": ["region", "comparison"], "max_item_count": 5, "default_item_count": 3, "allowed_nested_types": ["countries", "states", "cities"], "allow_nested_comparisons": true}'::jsonb)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO comparison_type_field_groups (type_id, group_id, category, label, position, is_default)
SELECT td.id, v.group_id, v.category, v.label, v.pos, v.is_default
FROM comparison_type_definitions td
JOIN (VALUES
    ('attractions',   '3ca5a3cb-5b85-4ad4-ae3b-4b3117253507'::uuid, 'overview',          'Overview',                      1,  true),
    ('attractions',   '64de3dd1-bd6d-41b4-974d-b6891d503864'::uuid, 'plan-your-visit',   'Plan Your Visit',                2,  false),
    ('attractions',   'c0ba8544-83ab-4a28-bb76-f91445b5e4e3'::uuid, 'history',           'History & Background',           3,  false),
    ('attractions',   '220f5ea4-5464-4ea2-8f11-7fe1b1a5b242'::uuid, 'facilities',        'Facilities & Accessibility',     4,  false),
    ('attractions',   'ca7d77a8-beff-4ad7-8216-1809ccd5bf45'::uuid, 'nearby',            'Nearby & Combine With',          5,  false),
    ('housing',       'a9bdc1cb-c68a-473e-b3d1-b0bf79fcb0c3'::uuid, NULL,                'Accommodation Guide Fields',    -1,  true),
    ('neighborhoods', 'a9e6f173-2367-475b-9c0a-f060521eacb6'::uuid, 'overview',          'Overview',                      1,  true),
    ('neighborhoods', 'e0d6d9ef-5937-4a55-8721-20e7d765074a'::uuid, 'safety',            'Safety',                        2,  false),
    ('neighborhoods', '520c64e8-902c-45e5-826e-56de4d292146'::uuid, 'rent-prices',       'Rent Prices',                   3,  false),
    ('neighborhoods', '5b866609-5ce8-4859-831a-1bedf7ed1424'::uuid, 'amenities',         'Amenities',                     4,  false),
    ('neighborhoods', 'fcfa8f5b-3e85-44e6-9b42-ae98a5c7cb88'::uuid, 'transport',         'Getting Around',                5,  false),
    ('neighborhoods', '68250a45-1544-4295-abf0-f9c4ef1784ed'::uuid, 'schools',           'Schools & Family',              6,  false),
    ('neighborhoods', '1ba70cc0-b23c-40f8-832e-3a609e138e30'::uuid, 'other',             'Other',                         7,  false),
    ('neighborhoods', 'df909160-124c-43b9-83f4-1a5a25df22e7'::uuid, 'demographics',      'Who Lives Here',                7,  false),
    ('regions',       '7ba78bcf-1a69-4f69-bb12-53d62c6fd677'::uuid, 'overview',          'Overview',                      1,  true),
    ('regions',       'b3eaca7e-9a7a-4850-b918-d95f3e436d60'::uuid, 'countries',         '{name} Countries',              2,  false),
    ('regions',       '3fa56f49-f279-4dc8-8f63-6ab9c3e2e5b3'::uuid, 'cost-of-living',    'Cost of Living',                3,  false),
    ('regions',       'bb6002f5-8f59-4841-8a7c-685168660da5'::uuid, 'universities',      '{name} Universities',           4,  false),
    ('regions',       'df128992-75d7-4a1a-bd72-e15912092a48'::uuid, 'attractions',       '{name} Attractions',            5,  false),
    ('regions',       '99136766-4293-45d1-8185-478e755deb7c'::uuid, 'embassies',         '{name} Embassies',              6,  false),
    ('regions',       '16d92687-fadd-47d4-9ec2-7060644f1b3b'::uuid, 'banking',           'Banking',                       7,  false),
    ('regions',       'fc92bf47-0982-42a1-8e33-f776fc459eb0'::uuid, 'scholarships',      '{name} Scholarships',           8,  false),
    ('regions',       '7bb258b6-bde1-4e3d-bfc7-0262195f7a35'::uuid, 'other',             'Other',                         9,  false)
) AS v(type_slug, group_id, category, label, pos, is_default) ON v.type_slug = td.slug
ON CONFLICT DO NOTHING;

COMMIT;
