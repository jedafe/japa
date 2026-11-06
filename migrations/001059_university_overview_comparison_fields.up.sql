-- 001059_university_overview_comparison_fields.up.sql
-- Make the destination-wiki university overview fields the dimension source
-- for 'universities' comparisons (§30 Sub-spec 5 + §80 field hydration).
--
-- The comparison type was bound to university_fields (…044) — a group that is
-- not in the university wiki schema (its space_type:university / space class
-- assignments stopped resolving after 000970 retired the 'university' space
-- type slug), so comparison values were not editable through wiki fields.
-- wiki_university_overview IS assigned to space_type:education (001035), so
-- its fields are editable via SuggestEdit / field ops / the fields panel.
--
-- This migration:
--   1. Repoints comparison_type_definitions.field_group_id for 'universities'
--      to wiki_university_overview — subsequent comparisons auto-generate
--      dimensions from the overview group's fields.
--   2. Aligns the overview group's keys/labels/types with university_fields
--      so dimension types resolve identically. Matrix cells hydrate by field
--      KEY from the entity's merged custom_field_values pool, so existing
--      comparisons (dims sourced from university_fields) keep hydrating from
--      the same keys — no dimension repair migration needed.
--   3. Renames the two near-matching overview keys. cfv rows reference
--      field_id, so renames surface existing wiki values under the
--      comparison key without touching custom_field_values.
--   4. Grants the standard wiki field permissions on the added fields.
--
-- Two overview-only fields (international_students_pct, notable_for) are kept
-- at the end — they become two extra auto-generated dimensions.
--
-- Known pre-existing ambiguity (unchanged by this migration): acceptance_rate
-- also exists in wiki_university_admissions, and founded_year /
-- student_population / acceptance_rate also exist in university_fields. cfv
-- reads merge last-row-wins per key; writes resolve first-assigned-group.

BEGIN;

-- 1. Subsequent 'universities' comparisons generate dimensions from the
--    overview group.
UPDATE comparison_type_definitions t
SET field_group_id = g.id
FROM custom_field_groups g
WHERE g.slug = 'wiki_university_overview'
  AND t.slug = 'universities';

-- 2. Align the two near-matching overview keys with university_fields.
UPDATE custom_fields f
SET key = 'type'
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'wiki_university_overview'
  AND f.key = 'institution_type';

UPDATE custom_fields f
SET key = 'official_website_url'
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'wiki_university_overview'
  AND f.key = 'official_website';

-- 3. Upsert the full dimension set — keys, labels, field types, flags and
--    positions mirror university_fields so the generated matrix reads
--    identically.
INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('initials',                  'Initials',                      'text',        NULL,         NULL, NULL, NULL, false, false, true, true,  false, true,  0,  NULL),
    ('official_website_url',      'Official Website',              'url',         NULL,         NULL, NULL, NULL, false, false, true, false, false, false, 1,  NULL),
    ('contact_email',             'Contact Email',                 'email',       NULL,         NULL, NULL, NULL, false, false, true, false, false, false, 2,  NULL),
    ('accreditation_body',        'Accreditation Body',            'text',        NULL,         NULL, NULL, NULL, false, false, true, false, false, true,  3,  NULL),
    ('world_ranking',             'World Ranking',                 'number',      NULL,         NULL, NULL, NULL, false, false, true, true,  true,  false, 4,  NULL),
    ('country_ranking',           'Country Ranking',               'number',      NULL,         NULL, NULL, NULL, false, false, true, true,  true,  false, 5,  NULL),
    ('founded_year',              'Founded Year',                  'number',      NULL,         NULL, NULL, NULL, false, false, true, false, true,  false, 6,  NULL),
    ('type',                      'Type',                          'select',      '[{"value":"public","label":"Public"},{"value":"private","label":"Private"}]', NULL, NULL, NULL, false, false, true, true, false, false, 7, NULL),
    ('programs_offered',          'Programs Offered',              'multiselect', '[]',         NULL, NULL, NULL, false, false, true, true,  false, false, 8,  NULL),
    ('tuition_range',             'Tuition Range',                 'select',      '[{"value":"free","label":"Free"},{"value":"<5k","label":"< $5k"},{"value":"5k-15k","label":"$5k - $15k"},{"value":"15k-30k","label":"$15k - $30k"},{"value":">30k","label":"> $30k"}]', NULL, NULL, NULL, false, false, true, true, false, false, 9, NULL),
    ('student_population',        'Student Population',            'number',      NULL,         NULL, NULL, NULL, false, false, true, false, true,  false, 10, NULL),
    ('acceptance_rate',           'Acceptance Rate (%)',           'number',      NULL,         NULL, NULL, NULL, false, false, true, false, true,  false, 11, NULL),
    ('international_support',     'International Student Support', 'boolean',     NULL,         NULL, NULL, NULL, false, false, true, true,  false, false, 12, NULL),
    ('scholarship_available',     'Scholarship Available',         'boolean',     NULL,         NULL, NULL, NULL, false, false, true, true,  false, false, 13, NULL),
    ('application_url',           'Application URL',               'url',         NULL,         NULL, NULL, NULL, false, false, true, false, false, false, 14, NULL),
    ('colleges',                  'Colleges / Faculties',          'text',        NULL,         NULL, NULL, NULL, false, false, true, false, false, false, 15, NULL),
    ('international_students_pct','International Students',        'percentage',  NULL,         NULL, NULL, NULL, false, false, true, false, false, false, 16, NULL),
    ('notable_for',               'Notable For',                   'text',        NULL,         NULL, NULL, NULL, false, false, true, false, false, false, 17, NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_university_overview'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

-- 4. Permissions for the added fields — view for every group, edit for every
--    non-guest group (000860/000883 convention).
INSERT INTO custom_field_permissions (field_id, user_group_id, can_view, can_edit)
SELECT f.id, ug.id, true, ug.slug <> 'guest'
FROM custom_fields f
JOIN custom_field_groups g ON g.id = f.group_id AND g.slug = 'wiki_university_overview'
CROSS JOIN user_groups ug
WHERE f.key IN ('initials', 'contact_email', 'accreditation_body',
    'world_ranking', 'country_ranking', 'programs_offered', 'tuition_range',
    'acceptance_rate', 'international_support', 'scholarship_available',
    'application_url', 'colleges')
ON CONFLICT (field_id, user_group_id) DO NOTHING;

COMMIT;
