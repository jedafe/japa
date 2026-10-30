-- 000972_wiki_programs_rollup_education.up.sql
-- Fixup for 000971: the university wiki type was replaced by 'education'
-- in 000970 (universities are level='university' rows under it), so the
-- 'programs' category rollup must target that type — 000971's UPDATE
-- against slug='university' matched no rows.
BEGIN;

UPDATE wiki_type_categories c
SET rollup = '{"enabled":true,"mode":"directory","levels":["course"]}'::jsonb
FROM space_types st
WHERE c.space_type_id = st.id AND st.slug IN ('education', 'university') AND c.slug = 'programs';

COMMIT;
