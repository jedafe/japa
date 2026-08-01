-- 000220_sidebar_widgets.down.sql
-- Reverse: rename sidebar_widgets → sidebar_blocks, widget_type → block_type.
-- Remove seeded default sidebar and widgets.

BEGIN;

-- Remove seeded default widgets
DELETE FROM sidebar_widgets
WHERE sidebar_id IN (
    SELECT id FROM sidebars
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
      AND app = 'main' AND position = 'right' AND slug = 'main-default-right'
);

-- Remove seeded default sidebar
DELETE FROM sidebars
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND app = 'main' AND position = 'right' AND slug = 'main-default-right';

-- Rename column back
ALTER TABLE sidebar_widgets RENAME COLUMN widget_type TO block_type;

-- Rename table back
ALTER TABLE sidebar_widgets RENAME TO sidebar_blocks;

-- Rename index back
ALTER INDEX IF EXISTS idx_sidebar_widgets_sidebar RENAME TO idx_sidebar_blocks_sidebar;

COMMIT;
