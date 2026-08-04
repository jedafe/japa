-- 000266_seed_compare_menu_item.down.sql
-- Removes the "Compare" item from the main sidebar.

BEGIN;

DELETE FROM menu_items
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND label = 'Compare'
  AND url = '/compare'
  AND EXISTS (
      SELECT 1 FROM menus m
      WHERE m.slug = 'main_sidebar'
        AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
        AND menu_items.menu_id = m.id
  );

COMMIT;
