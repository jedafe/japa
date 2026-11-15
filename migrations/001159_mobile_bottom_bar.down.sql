-- 001159_mobile_bottom_bar.down.sql — restores the pre-001159 seed: slug back
-- to 'main_bottom_bar' and the original five items.

BEGIN;

DELETE FROM menu_items
WHERE menu_id = (SELECT id FROM menus WHERE slug = 'mobile_bottom_bar');

INSERT INTO menu_items (menu_id, label, url, icon, sort_order, visibility, feature_flag)
SELECT m.id, v.label, v.url, v.icon, v.sort_order, v.visibility, v.feature_flag
FROM menus m
JOIN (VALUES
  ('Explore',       '/explore',       'compass', 0, 'all',           NULL::text),
  ('Home',          '/',              'home',    0, 'authenticated', NULL::text),
  ('Search',        '/search',        'search',  1, 'all',           NULL::text),
  ('Notifications', '/notifications', 'bell',    2, 'authenticated', 'feature.notifications'::text),
  ('Profile',       '/profile',       'user',    3, 'authenticated', NULL::text)
) AS v(label, url, icon, sort_order, visibility, feature_flag) ON true
WHERE m.slug = 'mobile_bottom_bar';

UPDATE menus SET slug = 'main_bottom_bar', updated_at = now()
WHERE slug = 'mobile_bottom_bar';

COMMIT;
