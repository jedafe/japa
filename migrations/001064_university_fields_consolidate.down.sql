-- 001064_university_fields_consolidate.down.sql
-- Reverse: recreate university_fields (…044), move the 16 canonical defs
-- back, rebind the repointed types/space type, recreate the overview dup
-- defs, and restore the admissions field key.
--
-- Approximation note: the overview defs dropped by the up migration were
-- auto-id'd in 001059 — this down recreates their schema (keys, labels,
-- types, options, positions) but not their UUIDs or permission rows' exact
-- linkage. cfv and dimension source_field_ids were never touched.

BEGIN;

-- 1. Recreate the group and its class assignments.
INSERT INTO custom_field_groups (id, name, slug, description, is_system)
VALUES ('00000000-0000-0000-0000-000000000044', 'University Fields', 'university_fields', NULL, true)
ON CONFLICT (id) DO NOTHING;

INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT '00000000-0000-0000-0000-000000000044', 'space', NULL, 0
WHERE NOT EXISTS (
    SELECT 1 FROM custom_field_group_assignments
    WHERE group_id = '00000000-0000-0000-0000-000000000044' AND entity_class = 'space' AND entity_id IS NULL
);

INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT '00000000-0000-0000-0000-000000000044', 'space_type:university', NULL, 0
WHERE NOT EXISTS (
    SELECT 1 FROM custom_field_group_assignments
    WHERE group_id = '00000000-0000-0000-0000-000000000044' AND entity_class = 'space_type:university' AND entity_id IS NULL
);

-- 2. Recreate the overview dup defs that were dropped (post-001059 schema),
--    then move the canonical defs back to …044.
INSERT INTO custom_fields
    (group_id, key, label, field_type, options, is_required, is_system,
     show_in_frontend, filterable, sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb,
       false, false, true, m.filterable, m.sortable, m.searchable, m.position, NULL
FROM custom_field_groups g
JOIN (VALUES
    ('initials',                 'Initials',                      'text',        NULL,  0),
    ('official_website_url',     'Official Website',              'url',         NULL,  1),
    ('contact_email',            'Contact Email',                 'email',       NULL,  2),
    ('accreditation_body',       'Accreditation Body',            'text',        NULL,  3),
    ('world_ranking',            'World Ranking',                 'number',      NULL,  4),
    ('country_ranking',          'Country Ranking',               'number',      NULL,  5),
    ('founded_year',             'Founded Year',                  'number',      NULL,  6),
    ('type',                     'Type',                          'select',      '[{"value":"public","label":"Public"},{"value":"private","label":"Private"}]', 7),
    ('programs_offered',         'Programs Offered',              'multiselect', '[]',  8),
    ('tuition_range',            'Tuition Range',                 'select',      '[{"value":"free","label":"Free"},{"value":"<5k","label":"< $5k"},{"value":"5k-15k","label":"$5k - $15k"},{"value":"15k-30k","label":"$15k - $30k"},{"value":">30k","label":"> $30k"}]', 9),
    ('student_population',       'Student Population',            'number',      NULL,  10),
    ('acceptance_rate',          'Acceptance Rate (%)',           'number',      NULL,  11),
    ('international_support',    'International Student Support', 'boolean',     NULL,  12),
    ('scholarship_available',    'Scholarship Available',         'boolean',     NULL,  13),
    ('application_url',          'Application URL',               'url',         NULL,  14),
    ('colleges',                 'Colleges / Faculties',          'text',        NULL,  15)
) AS m(key, label, field_type, options, position) ON true
WHERE g.slug = 'wiki_university_overview'
ON CONFLICT (group_id, key) DO NOTHING;

UPDATE custom_fields
SET group_id = '00000000-0000-0000-0000-000000000044'
WHERE group_id = (SELECT id FROM custom_field_groups WHERE slug = 'wiki_university_overview')
  AND key IN ('initials', 'official_website_url', 'contact_email',
      'accreditation_body', 'world_ranking', 'country_ranking', 'founded_year',
      'type', 'programs_offered', 'tuition_range', 'student_population',
      'acceptance_rate', 'international_support', 'scholarship_available',
      'application_url', 'colleges')
  -- only move the canonical (deterministic-id) defs, not the recreated dups
  AND id IN (
      '00000000-0000-0000-0000-000000000090','00000000-0000-0000-0000-000000000091',
      '00000000-0000-0000-0000-000000000092','00000000-0000-0000-0000-000000000093',
      '00000000-0000-0000-0000-000000000094','00000000-0000-0000-0000-000000000095',
      '00000000-0000-0000-0000-000000000096','00000000-0000-0000-0000-000000000097',
      '00000000-0000-0000-0000-000000000098','00000000-0000-0000-0000-000000000099',
      '00000000-0000-0000-0000-000000000100','00000000-0000-0000-0000-000000000101',
      '00000000-0000-0000-0000-000000000102','00000000-0000-0000-0000-000000000103',
      '00000000-0000-0000-0000-000000000104','00000000-0000-0000-0000-000000000105'
  );

-- 3. Rebind the types and space type to …044.
UPDATE comparison_type_definitions
SET field_group_id = '00000000-0000-0000-0000-000000000044'
WHERE slug IN ('courses', 'cost_of_living');

UPDATE space_types
SET field_group_id = '00000000-0000-0000-0000-000000000044'
WHERE slug = 'education';

-- 4. Restore the two overview-only fields' post-001059 positions.
UPDATE custom_fields f
SET position = 16
FROM custom_field_groups g
WHERE f.group_id = g.id AND g.slug = 'wiki_university_overview'
  AND f.key = 'international_students_pct';

UPDATE custom_fields f
SET position = 17
FROM custom_field_groups g
WHERE f.group_id = g.id AND g.slug = 'wiki_university_overview'
  AND f.key = 'notable_for';

-- 5. Restore the admissions field key.
UPDATE custom_fields f
SET key = 'acceptance_rate'
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'wiki_university_admissions'
  AND f.key = 'admissions_acceptance_rate';

COMMIT;
