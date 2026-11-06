-- 001067_scholarship_overview_guide_parity.down.sql
-- Restore the original 5-field wiki_scholarship_overview.

BEGIN;

DELETE FROM custom_field_permissions p
USING custom_fields f, custom_field_groups g
WHERE p.field_id = f.id AND f.group_id = g.id
  AND g.slug = 'wiki_scholarship_overview'
  AND f.key IN ('scholarship_name', 'provider_institution',
      'level_of_study', 'is_renewable', 'countries_eligible');

DELETE FROM custom_fields f
USING custom_field_groups g
WHERE f.group_id = g.id AND g.slug = 'wiki_scholarship_overview'
  AND f.key IN ('scholarship_name', 'provider_institution',
      'level_of_study', 'is_renewable', 'countries_eligible');

UPDATE custom_fields f
SET key = 'award_amount', label = 'Award Amount', field_type = 'text', position = 1
FROM custom_field_groups g
WHERE f.group_id = g.id AND g.slug = 'wiki_scholarship_overview'
  AND f.key = 'award_amount_usd';

UPDATE custom_fields f
SET key = 'coverage', label = 'Coverage', field_type = 'text', options = NULL, position = 2
FROM custom_field_groups g
WHERE f.group_id = g.id AND g.slug = 'wiki_scholarship_overview'
  AND f.key = 'coverage';

UPDATE custom_fields f
SET key = 'eligibility', label = 'Eligibility', field_type = 'textarea', position = 3
FROM custom_field_groups g
WHERE f.group_id = g.id AND g.slug = 'wiki_scholarship_overview'
  AND f.key = 'eligibility_criteria';

UPDATE custom_fields f
SET key = 'deadline', label = 'Deadline', field_type = 'text', position = 4
FROM custom_field_groups g
WHERE f.group_id = g.id AND g.slug = 'wiki_scholarship_overview'
  AND f.key = 'application_deadline';

UPDATE custom_fields f
SET position = 5
FROM custom_field_groups g
WHERE f.group_id = g.id AND g.slug = 'wiki_scholarship_overview'
  AND f.key = 'application_link';

COMMIT;
