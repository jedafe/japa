-- 000354_seed_v2_main_sidebar_menu_items.up.sql
-- Seeds the v2 community features (§§79–89, §98) and other v2 feature pages
-- (§§63–64, §74, §102) into the main_sidebar menu so they are discoverable on
-- apps/main. Previously every v2 feature page existed and called the backend
-- but had no nav entry, making them reachable only by direct URL.
-- Ref: JEOS §§60–102, ADR-058 (unified app shell), ADR-087–110 (v2 features).
-- Wiki (§80) is intentionally omitted: /wiki has no index page, only
-- /wiki/[space_slug] — a separate index page is needed before a nav entry.

BEGIN;

-- Helper: the default tenant + main_sidebar menu id.
-- All inserts are idempotent via WHERE NOT EXISTS on (menu_id, url).

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Story Wall', '/stories', 'book-open', 20, 'authenticated', '_self', 'active'
FROM menus m
WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
AND NOT EXISTS (SELECT 1 FROM menu_items mi WHERE mi.menu_id = m.id AND mi.url = '/stories');

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Achievements', '/achievements', 'award', 21, 'authenticated', '_self', 'active'
FROM menus m
WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
AND NOT EXISTS (SELECT 1 FROM menu_items mi WHERE mi.menu_id = m.id AND mi.url = '/achievements');

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Japa Stage', '/japa-stage', 'flag', 22, 'authenticated', '_self', 'active'
FROM menus m
WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
AND NOT EXISTS (SELECT 1 FROM menu_items mi WHERE mi.menu_id = m.id AND mi.url = '/japa-stage');

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Scam Alerts', '/scam-alerts', 'shield-alert', 23, 'authenticated', '_self', 'active'
FROM menus m
WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
AND NOT EXISTS (SELECT 1 FROM menu_items mi WHERE mi.menu_id = m.id AND mi.url = '/scam-alerts');

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Mutual Aid', '/mutual-aid', 'life-buoy', 24, 'authenticated', '_self', 'active'
FROM menus m
WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
AND NOT EXISTS (SELECT 1 FROM menu_items mi WHERE mi.menu_id = m.id AND mi.url = '/mutual-aid');

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Planning Groups', '/planning-groups', 'target', 25, 'authenticated', '_self', 'active'
FROM menus m
WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
AND NOT EXISTS (SELECT 1 FROM menu_items mi WHERE mi.menu_id = m.id AND mi.url = '/planning-groups');

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Calendar', '/calendar', 'calendar', 26, 'authenticated', '_self', 'active'
FROM menus m
WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
AND NOT EXISTS (SELECT 1 FROM menu_items mi WHERE mi.menu_id = m.id AND mi.url = '/calendar');

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Digest', '/digest', 'newspaper', 27, 'authenticated', '_self', 'active'
FROM menus m
WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
AND NOT EXISTS (SELECT 1 FROM menu_items mi WHERE mi.menu_id = m.id AND mi.url = '/digest');

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Ask an Expat', '/ama', 'message-square', 28, 'authenticated', '_self', 'active'
FROM menus m
WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
AND NOT EXISTS (SELECT 1 FROM menu_items mi WHERE mi.menu_id = m.id AND mi.url = '/ama');

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Services', '/services', 'building-2', 29, 'authenticated', '_self', 'active'
FROM menus m
WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
AND NOT EXISTS (SELECT 1 FROM menu_items mi WHERE mi.menu_id = m.id AND mi.url = '/services');

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Jobs', '/jobs', 'briefcase', 30, 'authenticated', '_self', 'active'
FROM menus m
WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
AND NOT EXISTS (SELECT 1 FROM menu_items mi WHERE mi.menu_id = m.id AND mi.url = '/jobs');

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Challenges', '/challenges', 'flame', 31, 'authenticated', '_self', 'active'
FROM menus m
WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
AND NOT EXISTS (SELECT 1 FROM menu_items mi WHERE mi.menu_id = m.id AND mi.url = '/challenges');

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Leaderboard', '/leaderboard', 'trophy', 32, 'authenticated', '_self', 'active'
FROM menus m
WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
AND NOT EXISTS (SELECT 1 FROM menu_items mi WHERE mi.menu_id = m.id AND mi.url = '/leaderboard');

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Mentors', '/mentors', 'heart-handshake', 33, 'authenticated', '_self', 'active'
FROM menus m
WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
AND NOT EXISTS (SELECT 1 FROM menu_items mi WHERE mi.menu_id = m.id AND mi.url = '/mentors');

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Remittance', '/tools/remittance', 'send', 34, 'authenticated', '_self', 'active'
FROM menus m
WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
AND NOT EXISTS (SELECT 1 FROM menu_items mi WHERE mi.menu_id = m.id AND mi.url = '/tools/remittance');

COMMIT;
