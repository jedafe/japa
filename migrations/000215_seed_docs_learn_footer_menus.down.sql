-- 000215_seed_docs_learn_footer_menus.down.sql
BEGIN;
DELETE FROM menus
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug IN ('docs_footer', 'learn_footer');
COMMIT;
