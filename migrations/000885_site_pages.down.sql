-- 000885_site_pages.down.sql
-- Removes the seeded site pages, the contact form (and its fields), and
-- the footer links added by this migration. The 'feedback'/'report' form
-- status flip is intentionally not reversed — 'published' is the correct
-- state for public embeds; restoring 'active' would re-break /feedback.

BEGIN;

DELETE FROM pages WHERE slug IN ('about', 'contact', 'faq', 'privacy', 'terms', 'rules', 'cookies', 'safety');

DELETE FROM menu_items
WHERE menu_id = (SELECT id FROM menus WHERE slug = 'main_footer')
  AND url IN ('/contact', '/faq', '/rules', '/cookies', '/safety');

UPDATE menu_items SET sort_order = 7 WHERE menu_id = (SELECT id FROM menus WHERE slug = 'main_footer') AND url = '/signup';
UPDATE menu_items SET sort_order = 8 WHERE menu_id = (SELECT id FROM menus WHERE slug = 'main_footer') AND url = '/login';

DELETE FROM custom_field_permissions WHERE field_id IN (
	SELECT id FROM custom_fields WHERE group_id = '00000000-0000-0000-0000-0000000000c2' OR id = '00000000-0000-0000-0000-0000000000d5'
);
DELETE FROM custom_field_group_assignments WHERE group_id = '00000000-0000-0000-0000-0000000000c2' AND entity_class = 'form';
DELETE FROM custom_fields WHERE group_id = '00000000-0000-0000-0000-0000000000c2' OR id = '00000000-0000-0000-0000-0000000000d5';
DELETE FROM custom_field_groups WHERE id = '00000000-0000-0000-0000-0000000000c2';
DELETE FROM forms WHERE slug = 'contact';

COMMIT;
