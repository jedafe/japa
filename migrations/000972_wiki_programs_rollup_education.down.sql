BEGIN;

UPDATE wiki_type_categories c
SET rollup = NULL
FROM space_types st
WHERE c.space_type_id = st.id AND st.slug IN ('education', 'university') AND c.slug = 'programs';

COMMIT;
