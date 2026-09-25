-- 000213_seed_all_japaconnect_menus.down.sql
-- Reverses 000213: removes seeded menus/items and restores original placement constraint.
-- Note: only removes the menus seeded by 000213; main_sidebar and main_bottom_bar
-- (seeded by 000136) are left intact.

BEGIN;

-- Remove the menus (cascade deletes their menu_items).
DELETE FROM menus
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug IN (
    'main_user_dropdown',
    'main_footer',
    'docs_sidebar',
    'docs_user_dropdown',
    'learn_sidebar',
    'learn_user_dropdown',
    'learn_bottom_bar',
    'auth_footer'
  );

-- Restore the original placement CHECK (sidebar, bottom_bar only).
ALTER TABLE menus DROP CONSTRAINT IF EXISTS menus_placement_check;
ALTER TABLE menus ADD CONSTRAINT menus_placement_check
    CHECK (placement IN ('sidebar', 'bottom_bar'));

COMMIT;
