-- 001119_faq_sidenav.down.sql
-- Restores the /faq page's category tabs to the horizontal pill row.

BEGIN;

UPDATE pages
SET blocks = jsonb_set(blocks, '{1,props,orientation}', '"horizontal"'::jsonb, true),
    updated_at = now()
WHERE slug = 'faq'
  AND blocks->1->>'type' = 'tabs';

COMMIT;
