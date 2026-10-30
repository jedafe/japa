-- 000971_wiki_program_type_related_mode.down.sql
BEGIN;

UPDATE wiki_type_categories c
SET rollup = NULL
FROM space_types st
WHERE c.space_type_id = st.id AND st.slug = 'university' AND c.slug = 'programs';

DELETE FROM custom_field_group_assignments a
USING custom_field_groups g
WHERE a.group_id = g.id
  AND g.slug IN ('wiki_program_overview','wiki_program_universities',
                 'wiki_program_career_outcomes','wiki_course_universities');

DELETE FROM wiki_type_categories c
USING space_types st
WHERE c.space_type_id = st.id
  AND ((st.slug = 'program') OR (st.slug = 'course' AND c.slug = 'universities'));

DELETE FROM custom_field_groups
WHERE slug IN ('wiki_program_overview','wiki_program_universities',
               'wiki_program_career_outcomes','wiki_course_universities');

DELETE FROM wiki_type_configs WHERE space_type_id = '00000000-0000-0000-0000-00000000007e';
DELETE FROM space_types WHERE id = '00000000-0000-0000-0000-00000000007e';
DELETE FROM wiki_entity_kinds WHERE entity_kind = 'program';

COMMIT;
