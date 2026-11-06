-- 001065_scholarships_type_guide_fields.down.sql
-- Reverse: rebind 'scholarships' to wiki_scholarship_overview and repoint the
-- repaired dimensions back to their overview source fields.

BEGIN;

UPDATE comparison_type_definitions t
SET field_group_id = g.id
FROM custom_field_groups g
WHERE g.slug = 'wiki_scholarship_overview'
  AND t.slug = 'scholarships';

UPDATE comparison_dimensions d
SET source_field_id = odef.id
FROM custom_fields gdef
JOIN custom_field_groups gg ON gg.id = gdef.group_id AND gg.slug = 'guide_scholarship_fields'
JOIN custom_fields odef ON odef.key = gdef.key
JOIN custom_field_groups og ON og.id = odef.group_id AND og.slug = 'wiki_scholarship_overview'
WHERE d.source_field_id = gdef.id
  AND gdef.key IN ('coverage', 'application_link');

UPDATE comparison_dimensions d
SET source_field_id = odef.id
FROM custom_fields gdef
JOIN custom_field_groups gg ON gg.id = gdef.group_id AND gg.slug = 'guide_scholarship_fields'
JOIN (VALUES
    ('award_amount_usd',        'award_amount'),
    ('eligibility_criteria',    'eligibility'),
    ('application_deadline',    'deadline')
) AS m(new_key, old_key) ON m.new_key = gdef.key
JOIN custom_fields odef ON odef.key = m.old_key
JOIN custom_field_groups og ON og.id = odef.group_id AND og.slug = 'wiki_scholarship_overview'
WHERE d.source_field_id = gdef.id;

COMMIT;
