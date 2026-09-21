-- 000680_docs_footer_and_api_key_limit_fixes.down.sql
-- Reverts the docs_footer URL fixes and the is_public flag change.

BEGIN;

UPDATE menu_items mi
SET url = '/docs/getting-started'
FROM menus m
WHERE mi.menu_id = m.id
  AND m.slug = 'docs_footer'
  AND mi.url = '/docs/users'
  AND mi.label = 'Getting Started';

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT m.tenant_id, m.id, 'Guides', '/docs/guides', NULL, 3, 'all', '_self', 'active'
FROM menus m
WHERE m.slug = 'docs_footer'
  AND NOT EXISTS (
      SELECT 1 FROM menu_items mi WHERE mi.menu_id = m.id AND mi.label = 'Guides'
  );

UPDATE system_settings
SET is_public = false
WHERE key = 'api.keys.max_per_user';

COMMIT;
