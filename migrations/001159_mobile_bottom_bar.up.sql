-- 001159_mobile_bottom_bar.up.sql
-- §23 Mobile bottom bar + ADR-128: the frontend contract is GET
-- /menus/mobile_bottom_bar (apps/main layout, Flutter shell) but the seeded
-- menu was slugged 'main_bottom_bar', so every consumer silently fell back to
-- its built-in tabset and admin edits never reached clients. Rename the menu
-- to the contract slug and reshape its items to the §23 tab set
-- (Home · Spaces · Compose · Notifications · Profile). The literal url
-- 'compose' is the established compose-slot marker MobileBottomTabBar already
-- renders as the composer button; '/notifications' carries the unread badge.
-- Guest rows keep parity with the previous web fallback (Home·Search·Log in).

BEGIN;

UPDATE menus SET slug = 'mobile_bottom_bar', updated_at = now()
WHERE slug = 'main_bottom_bar';

DELETE FROM menu_items
WHERE menu_id = (SELECT id FROM menus WHERE slug = 'mobile_bottom_bar');

INSERT INTO menu_items (menu_id, label, url, icon, sort_order, visibility, feature_flag)
SELECT m.id, v.label, v.url, v.icon, v.sort_order, v.visibility, v.feature_flag
FROM menus m
JOIN (VALUES
  ('Home',          '/',              'home',   0, 'all',           NULL::text),
  ('Spaces',        '/spaces',        'users',  1, 'authenticated', NULL::text),
  ('Compose',       'compose',        NULL,     2, 'authenticated', NULL::text),
  ('Notifications', '/notifications', 'bell',   3, 'authenticated', 'feature.notifications'::text),
  ('Profile',       '/profile',       'user',   4, 'authenticated', NULL::text),
  ('Search',        '/search',        'search', 5, 'guest',         NULL::text),
  ('Log in',        '/login',         'log-in', 6, 'guest',         NULL::text)
) AS v(label, url, icon, sort_order, visibility, feature_flag) ON true
WHERE m.slug = 'mobile_bottom_bar';

COMMIT;
