-- 000910_wiki_category_name_tokens.down.sql
-- Reverses 000910: strips the '{name} ' prefix this migration added.

BEGIN;

UPDATE wiki_type_categories c
SET label = substring(c.label from '^\{name\} (.*)$')
FROM space_types st
WHERE c.space_type_id = st.id
  AND st.slug IN ('country','state','city')
  AND c.slug IN ('visa-routes','embassies','universities','attractions','schools')
  AND c.label LIKE '{name} %';

COMMIT;
