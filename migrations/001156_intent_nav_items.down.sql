-- 001156_intent_nav_items.down.sql — drop the intent-nav seed items.

BEGIN;

DELETE FROM menu_items mi
USING menus m
WHERE mi.menu_id = m.id
  AND m.slug = 'main_sidebar'
  AND mi.url IN (
	'/universities',
	'/scholarships',
	'/wiki',
	'/tools/travel',
	'/tools/community-locator',
	'/tools/credential-checker',
	'/tools/language-test',
	'/tools/cv-formatter',
	'/tools/tax-calculator'
  );

COMMIT;
