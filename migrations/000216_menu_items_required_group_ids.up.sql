-- 000216_menu_items_required_group_ids.up.sql
-- Replaces permission_node gating with user-group-based gating on menu items.
-- Admins select which user groups should see a menu item; the item is shown
-- if the user belongs to ANY of the required groups. An empty array means
-- the item is visible to everyone (filtered only by visibility).
-- Ref: ADR-054 (Navigation Menus)

BEGIN;

ALTER TABLE menu_items
    ADD COLUMN IF NOT EXISTS required_group_ids jsonb NOT NULL DEFAULT '[]'::jsonb;

-- Validate that required_group_ids is an array of UUID strings (or empty).
ALTER TABLE menu_items
    ADD CONSTRAINT menu_items_required_group_ids_is_array
    CHECK (jsonb_typeof(required_group_ids) = 'array');

COMMIT;
