-- 000910_wiki_category_name_tokens.up.sql
-- {name} tokens in wiki category labels (review): category labels now run
-- the same substituteTemplateLabel pipeline as page-template labels, so a
-- category can read "{name} Visa Routes" and render "Argentina Visa
-- Routes" on the country's index/page/tab strip. Update the seeded
-- geo-level categories that read better host-scoped.

BEGIN;

UPDATE wiki_type_categories c
SET label = '{name} ' || c.label
FROM space_types st
WHERE c.space_type_id = st.id
  AND st.slug IN ('country','state','city')
  AND c.slug IN ('visa-routes','embassies','universities','attractions','schools')
  AND c.label NOT LIKE '{name}%';

COMMIT;
