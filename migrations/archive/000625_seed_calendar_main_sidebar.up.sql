-- 000625_seed_calendar_main_sidebar.up.sql
-- Ensure the Community Calendar (§86) has a discoverable main-sidebar entry.
-- The v2 feature menu seed (000354) included Calendar, but some environments
-- were provisioned without it or had it removed during later menu rebuilds.
-- Ref: japaconnect-engineering/sections/86-community-calendar.md

BEGIN;

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Calendar', '/calendar', 'calendar', 26, 'authenticated', '_self', 'active'
FROM menus m
WHERE m.slug = 'main_sidebar' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
AND NOT EXISTS (SELECT 1 FROM menu_items mi WHERE mi.menu_id = m.id AND mi.url = '/calendar');

COMMIT;
