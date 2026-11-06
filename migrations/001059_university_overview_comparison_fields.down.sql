-- 001059_university_overview_comparison_fields.down.sql
-- Reverse: rebind 'universities' to university_fields (…044), drop the fields
-- added to wiki_university_overview, and restore the original six overview
-- fields (keys, labels, options, positions from 001035).

BEGIN;

UPDATE comparison_type_definitions
SET field_group_id = '00000000-0000-0000-0000-000000000044'::uuid
WHERE slug = 'universities';

-- cfv rows for fields this migration added have no pre-migration home —
-- remove them with the field rows.
DELETE FROM custom_field_values v
USING custom_fields f, custom_field_groups g
WHERE v.field_id = f.id
  AND f.group_id = g.id
  AND g.slug = 'wiki_university_overview'
  AND f.key IN ('initials', 'contact_email', 'accreditation_body',
      'world_ranking', 'country_ranking', 'programs_offered', 'tuition_range',
      'acceptance_rate', 'international_support', 'scholarship_available',
      'application_url', 'colleges');

DELETE FROM custom_field_permissions p
USING custom_fields f, custom_field_groups g
WHERE p.field_id = f.id
  AND f.group_id = g.id
  AND g.slug = 'wiki_university_overview'
  AND f.key IN ('initials', 'contact_email', 'accreditation_body',
      'world_ranking', 'country_ranking', 'programs_offered', 'tuition_range',
      'acceptance_rate', 'international_support', 'scholarship_available',
      'application_url', 'colleges');

DELETE FROM custom_fields f
USING custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'wiki_university_overview'
  AND f.key IN ('initials', 'contact_email', 'accreditation_body',
      'world_ranking', 'country_ranking', 'programs_offered', 'tuition_range',
      'acceptance_rate', 'international_support', 'scholarship_available',
      'application_url', 'colleges');

-- Restore renamed keys and the original labels/options/positions.
UPDATE custom_fields f
SET key = 'institution_type', label = 'Institution Type',
    options = '["Public","Private"]'::jsonb, position = 2
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'wiki_university_overview'
  AND f.key = 'type';

UPDATE custom_fields f
SET key = 'official_website', label = 'Official Website', position = 5
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'wiki_university_overview'
  AND f.key = 'official_website_url';

UPDATE custom_fields f
SET label = 'Founded', position = 1
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'wiki_university_overview'
  AND f.key = 'founded_year';

UPDATE custom_fields f
SET position = 3
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'wiki_university_overview'
  AND f.key = 'student_population';

UPDATE custom_fields f
SET position = 4
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'wiki_university_overview'
  AND f.key = 'international_students_pct';

UPDATE custom_fields f
SET position = 6
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'wiki_university_overview'
  AND f.key = 'notable_for';

COMMIT;
