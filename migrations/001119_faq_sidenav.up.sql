-- 001119_faq_sidenav.up.sql
-- Renders the /faq page's category tabs as a left sidenav (vertical
-- orientation) instead of a horizontal pill row. The tabs block props
-- gain orientation='vertical'; TabsBlock renders vertical as a sidenav.
--
-- Ref: .jeos/sections/16-page-builder.md (tabs block props)

BEGIN;

UPDATE pages
SET blocks = jsonb_set(blocks, '{1,props,orientation}', '"vertical"'::jsonb, true),
    updated_at = now()
WHERE slug = 'faq'
  AND blocks->1->>'type' = 'tabs';

-- The published-page cache (jc:page:faq) expires on its own TTL; callers
-- that need it immediately should DEL the key after migrating.

COMMIT;
