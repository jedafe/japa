-- 000908_wiki_entity_field_perms.down.sql
-- Reverses 000908: removes the permission grants on the 000903 overview
-- fields. Fields added by other migrations under the same groups would
-- also lose grants — acceptable since these groups' fields all originate
-- from 000903.

BEGIN;

DELETE FROM custom_field_permissions fp
USING custom_fields f
JOIN custom_field_groups g ON g.id = f.group_id
WHERE fp.field_id = f.id
  AND g.slug IN (
    'wiki_visa_route_overview', 'wiki_student_visa_overview',
    'wiki_work_visa_overview', 'wiki_family_visa_overview',
    'wiki_tourist_visa_overview', 'wiki_business_visa_overview',
    'wiki_pr_pathway_overview',
    'wiki_course_overview', 'wiki_scholarship_overview',
    'wiki_bank_overview', 'wiki_embassy_overview',
    'wiki_neighborhood_overview'
  );

COMMIT;
