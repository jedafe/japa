-- 000354_seed_v2_main_sidebar_menu_items.down.sql
-- Removes the v2 feature menu items seeded by the up migration.

BEGIN;

DELETE FROM menu_items
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND url IN (
    '/stories', '/achievements', '/japa-stage', '/scam-alerts', '/mutual-aid',
    '/planning-groups', '/calendar', '/digest', '/ama', '/services',
    '/jobs', '/challenges', '/leaderboard', '/mentors', '/tools/remittance'
  )
  AND EXISTS (
      SELECT 1 FROM menus m
      WHERE m.slug = 'main_sidebar'
        AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
        AND menu_items.menu_id = m.id
  );

COMMIT;
