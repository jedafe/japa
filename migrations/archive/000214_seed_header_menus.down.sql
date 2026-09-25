-- 000214_seed_header_menus.down.sql
-- Reverses 000214: removes seeded header menus and restores placement constraint
-- without 'header'.

BEGIN;

DELETE FROM menus
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug IN ('main_header', 'learn_header', 'docs_header');

ALTER TABLE menus DROP CONSTRAINT IF EXISTS menus_placement_check;
ALTER TABLE menus ADD CONSTRAINT menus_placement_check
    CHECK (placement IN ('sidebar', 'bottom_bar', 'dropdown', 'footer'));

COMMIT;
