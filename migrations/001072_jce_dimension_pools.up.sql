-- 001072_jce_dimension_pools.up.sql
-- JCE Sub-spec 6: multi-group dimension pools.
--
-- Seeding: bound group always joins the pool as is_default. Wiki-bound types
-- additionally get every category group of their wiki type via an explicit
-- comparison-type-slug → space_type-slug map (the bound group alone cannot
-- locate the wiki type: geo types bind shared non-category 'geo_fields',
-- COL subtypes bind non-category 'wiki_city_prices_*' groups).

BEGIN;

CREATE TABLE comparison_type_field_groups (
    type_id    uuid NOT NULL REFERENCES comparison_type_definitions(id) ON DELETE CASCADE,
    group_id   uuid NOT NULL REFERENCES custom_field_groups(id) ON DELETE RESTRICT,
    category   text,
    label      text,
    position   int  NOT NULL DEFAULT 0,
    is_default boolean NOT NULL DEFAULT false,
    PRIMARY KEY (type_id, group_id)
);
CREATE INDEX idx_comparison_type_field_groups_group
    ON comparison_type_field_groups (group_id);

-- 1. Wiki-bound types: every category group of the mapped wiki type.
--    is_default marks whichever row is the type's current bound group.
INSERT INTO comparison_type_field_groups (type_id, group_id, category, label, position, is_default)
SELECT td.id, wtc.field_group_id, wtc.slug, wtc.label, wtc.position,
       (wtc.field_group_id = td.field_group_id)
FROM comparison_type_definitions td
JOIN (VALUES
    ('countries',        'country'),
    ('states',           'state'),
    ('cities',           'city'),
    ('neighborhoods',    'neighborhood'),
    ('universities',     'education'),
    ('courses',          'course'),
    ('scholarships',     'scholarship'),
    ('banks',            'bank'),
    ('embassies',        'embassy'),
    ('visa-types',       'visa_route'),
    ('visa-routes',      'visa_route'),
    ('student-visas',    'student_visa'),
    ('work-visas',       'work_visa'),
    ('family-visas',     'family_visa'),
    ('tourist-visas',    'tourist_visa'),
    ('business-visas',   'business_visa'),
    ('pr-pathways',      'pr_pathway')
    -- NB: col-* subtypes are intentionally NOT mapped to 'city' — their
    -- subject is a single prices group (restaurants/utilities/…), so the
    -- pool stays bound-group-only via step 2 below. 001125 trims the
    -- over-seeded rows this mapping previously produced.
) AS m(type_slug, space_slug) ON m.type_slug = td.slug
JOIN space_types st ON st.slug = m.space_slug
JOIN wiki_type_categories wtc ON wtc.space_type_id = st.id
ON CONFLICT DO NOTHING;

-- 2. Bound group as pool member for any type not covered above
--    (non-wiki types: mentors, remittance, job-listings, …) and for bound
--    groups outside the category set (geo_fields on the geo types).
INSERT INTO comparison_type_field_groups (type_id, group_id, label, position, is_default)
SELECT td.id, td.field_group_id, g.name, -1, true
FROM comparison_type_definitions td
JOIN custom_field_groups g ON g.id = td.field_group_id
WHERE td.field_group_id IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM comparison_type_field_groups p
      WHERE p.type_id = td.id AND p.group_id = td.field_group_id
  )
ON CONFLICT DO NOTHING;

COMMIT;
