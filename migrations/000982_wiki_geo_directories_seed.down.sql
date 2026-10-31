-- 000982_wiki_geo_directories_seed.down.sql
-- Removes the geo directory categories seeded by the up migration and their
-- dedicated (empty) field groups + class assignments.

DELETE FROM wiki_type_categories
WHERE (space_type_id, slug) IN (
    ('00000000-0000-0000-0000-000000000060', 'states'),
    ('00000000-0000-0000-0000-000000000060', 'cities'),
    ('00000000-0000-0000-0000-000000000060', 'universities'),
    ('00000000-0000-0000-0000-000000000061', 'cities'),
    ('00000000-0000-0000-0000-000000000061', 'universities'),
    ('00000000-0000-0000-0000-000000000062', 'universities')
);

DELETE FROM custom_field_group_assignments a
USING custom_field_groups g
WHERE a.group_id = g.id AND g.slug IN (
    'wiki_country_states', 'wiki_country_cities', 'wiki_country_universities',
    'wiki_state_cities', 'wiki_state_universities', 'wiki_city_universities'
);

DELETE FROM custom_field_groups
WHERE slug IN (
    'wiki_country_states', 'wiki_country_cities', 'wiki_country_universities',
    'wiki_state_cities', 'wiki_state_universities', 'wiki_city_universities'
);
