-- 000908_wiki_entity_field_perms.up.sql
-- JCE ↔ wiki mapping review fix: 000903 seeded field schemas but no
-- custom_field_permissions rows — field-level RBAC defaults fail-closed,
-- so every non-manage user (including guests) saw the schema with values
-- stripped. Grant the canonical set used on existing overview fields:
-- guest → can_view only; every other standard group → can_view + can_edit
-- (community edits flow through the wiki revision/ops path anyway, so
-- can_edit gates the ops submit, not a direct write).

BEGIN;

INSERT INTO custom_field_permissions (field_id, user_group_id, can_view, can_edit)
SELECT f.id, ug.id,
       true,
       ug.slug <> 'guest'
FROM custom_fields f
JOIN custom_field_groups g ON g.id = f.group_id
CROSS JOIN user_groups ug
WHERE g.slug IN (
    'wiki_visa_route_overview', 'wiki_student_visa_overview',
    'wiki_work_visa_overview', 'wiki_family_visa_overview',
    'wiki_tourist_visa_overview', 'wiki_business_visa_overview',
    'wiki_pr_pathway_overview',
    'wiki_course_overview', 'wiki_scholarship_overview',
    'wiki_bank_overview', 'wiki_embassy_overview',
    'wiki_neighborhood_overview'
)
  AND ug.slug IN ('guest','members','pro','business','organization',
                  'admin-group','blog-moderator','forum-moderators','super_admin')
ON CONFLICT (field_id, user_group_id) DO NOTHING;

COMMIT;
