-- 000597_dashboard_rail_menu.down.sql
-- Reverts the dashboard_rail placement and removes the default dashboard rail
-- menu and its items.

BEGIN;

-- Remove the default dashboard rail menu and its items.
DELETE FROM menu_items
WHERE menu_id IN (
    SELECT id FROM menus
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'dashboard_rail'
);

DELETE FROM menus
WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'dashboard_rail';

-- Restore the placement check constraint to the original two values.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'menus'::regclass
          AND contype = 'c'
          AND pg_get_constraintdef(oid) LIKE '%placement%'
    ) THEN
        ALTER TABLE menus DROP CONSTRAINT menus_placement_check;
    END IF;
END $$;

ALTER TABLE menus
    ADD CONSTRAINT menus_placement_check
    CHECK (placement IN ('sidebar', 'bottom_bar'));

COMMIT;
