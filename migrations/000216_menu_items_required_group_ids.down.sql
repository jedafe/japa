-- 000216_menu_items_required_group_ids.down.sql
BEGIN;
ALTER TABLE menu_items DROP CONSTRAINT IF EXISTS menu_items_required_group_ids_is_array;
ALTER TABLE menu_items DROP COLUMN IF EXISTS required_group_ids;
COMMIT;
