-- Restore the three COL pool rows with their original positions
-- (cities@2, countries@3, states@2).

BEGIN;

INSERT INTO comparison_type_field_groups (type_id, group_id, category, label, position, is_default)
SELECT td.id, g.id, NULL, g.name, v.pos, false
FROM (VALUES
    ('cities',    'wiki_city_cost_of_living',    2),
    ('countries', 'wiki_country_cost_of_living', 3),
    ('states',    'wiki_state_cost_of_living',   2)
) AS v(type_slug, group_slug, pos)
JOIN comparison_type_definitions td ON td.slug = v.type_slug
JOIN custom_field_groups g ON g.slug = v.group_slug
ON CONFLICT DO NOTHING;

COMMIT;
