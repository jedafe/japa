-- 000903_wiki_entity_overview_fields.down.sql
-- Reverses 000903: removes the baseline field schemas seeded into the new
-- entity-bound overview groups. Only fields created by the up migration are
-- deleted — any fields added later under the same keys would also match;
-- acceptable because these keys were introduced by this migration.

BEGIN;

DELETE FROM custom_fields f
USING custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug IN (
      'wiki_visa_route_overview', 'wiki_student_visa_overview',
      'wiki_work_visa_overview', 'wiki_family_visa_overview',
      'wiki_tourist_visa_overview', 'wiki_business_visa_overview',
      'wiki_pr_pathway_overview'
  )
  AND f.key IN ('route_type','processing_time','application_fee','eligibility','official_link');

DELETE FROM custom_fields f
USING custom_field_groups g
WHERE f.group_id = g.id AND g.slug = 'wiki_course_overview'
  AND f.key IN ('degree_type','duration','tuition_note','entry_requirements','career_outcomes');

DELETE FROM custom_fields f
USING custom_field_groups g
WHERE f.group_id = g.id AND g.slug = 'wiki_scholarship_overview'
  AND f.key IN ('award_amount','coverage','eligibility','deadline','application_link');

DELETE FROM custom_fields f
USING custom_field_groups g
WHERE f.group_id = g.id AND g.slug = 'wiki_bank_overview'
  AND f.key IN ('bank_type','account_requirements','maintenance_fees','international_features','website');

DELETE FROM custom_fields f
USING custom_field_groups g
WHERE f.group_id = g.id AND g.slug = 'wiki_embassy_overview'
  AND f.key IN ('services','appointment_link','address','phone');

DELETE FROM custom_fields f
USING custom_field_groups g
WHERE f.group_id = g.id AND g.slug = 'wiki_neighborhood_overview'
  AND f.key IN ('character','safety_note','typical_rent','walkability');

COMMIT;
