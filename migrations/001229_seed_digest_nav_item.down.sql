-- 001229_seed_digest_nav_item.down.sql

BEGIN;

DELETE FROM menu_items
WHERE menu_id IN (SELECT id FROM menus WHERE slug = 'main_sidebar')
  AND url = '/digest'
  AND feature_flag = 'digest.enabled';

COMMIT;
