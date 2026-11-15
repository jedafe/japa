-- 001153_become_a_partner_page.down.sql — reverses 001153.

BEGIN;

-- 1. Restore Organisation type options (001152 set — without Travel agency).
UPDATE custom_fields
SET options =
  '[{"value":"university","label":"University or school"},{"value":"employer","label":"Employer"},{"value":"community_org","label":"Community organisation"},{"value":"media","label":"Media or press"},{"value":"government_agency","label":"Government agency"},{"value":"other","label":"Other"}]'::jsonb
WHERE id = '00000000-0000-0000-0000-000000000f32';

-- 2. Drop the partners page.
DELETE FROM pages WHERE slug = 'partners';

-- 3a. /contact card back to the bare form renderer (001152 state).
UPDATE pages
SET blocks = jsonb_set(blocks, '{1,props,cards,3,cta}',
		'{"label": "Partner enquiry", "href": "/forms/partner-enquiry", "variant": "link"}'::jsonb, false),
	updated_at = now()
WHERE slug = 'contact'
  AND blocks->1->'props'->'cards'->3->>'title' = 'Partnerships & press';

-- 3b. Remove the footer link and restore prior sort orders.
DELETE FROM menu_items
WHERE menu_id = (SELECT id FROM menus WHERE slug = 'main_footer') AND url = '/partners';

UPDATE menu_items SET sort_order = 12 WHERE menu_id = (SELECT id FROM menus WHERE slug = 'main_footer') AND url = '/feedback';
UPDATE menu_items SET sort_order = 13 WHERE menu_id = (SELECT id FROM menus WHERE slug = 'main_footer') AND url = '/signup';
UPDATE menu_items SET sort_order = 14 WHERE menu_id = (SELECT id FROM menus WHERE slug = 'main_footer') AND url = '/login';

COMMIT;
