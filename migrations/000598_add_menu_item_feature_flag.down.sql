-- 000598_add_menu_item_feature_flag.down.sql
BEGIN;

ALTER TABLE menu_items DROP COLUMN IF EXISTS feature_flag;

COMMIT;
