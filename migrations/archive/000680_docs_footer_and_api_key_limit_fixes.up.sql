-- 000680_docs_footer_and_api_key_limit_fixes.up.sql
-- 1. Fix stale docs_footer menu items: /docs/getting-started and /docs/guides
--    are not valid audience slugs, so they redirect to /docs/access-denied.
--    Repoint "Getting Started" at the users audience landing (the only seeded
--    section with real pages) and drop "Guides" (no guides section exists).
-- 2. Expose api.keys.max_per_user via GET /site/settings/public — the docs
--    API keys page reads it to show/enforce the active-key limit (§47).
-- Ref: ADR-054 (menus), ADR-060 (docs surface).

BEGIN;

UPDATE menu_items mi
SET url = '/docs/users'
FROM menus m
WHERE mi.menu_id = m.id
  AND m.slug = 'docs_footer'
  AND mi.url = '/docs/getting-started';

DELETE FROM menu_items mi
USING menus m
WHERE mi.menu_id = m.id
  AND m.slug = 'docs_footer'
  AND mi.url = '/docs/guides';

UPDATE system_settings
SET is_public = true
WHERE key = 'api.keys.max_per_user';

COMMIT;
