-- Restore the full 'city' category pool for the col-* subtypes, as
-- 001072 originally seeded it.
BEGIN;

INSERT INTO comparison_type_field_groups (type_id, group_id, category, label, position, is_default)
SELECT td.id, wtc.field_group_id, wtc.slug, wtc.label, wtc.position,
       (wtc.field_group_id = td.field_group_id)
FROM comparison_type_definitions td
JOIN (VALUES
    ('cost_of_living',    'city'),
    ('col-transportation','city'),
    ('col-restaurants',   'city'),
    ('col-food-groceries','city'),
    ('col-utilities',     'city'),
    ('col-lifestyle',     'city')
) AS m(type_slug, space_slug) ON m.type_slug = td.slug
JOIN space_types st ON st.slug = m.space_slug
JOIN wiki_type_categories wtc ON wtc.space_type_id = st.id
ON CONFLICT DO NOTHING;

COMMIT;
