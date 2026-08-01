-- 000220_sidebar_widgets.up.sql
-- Rename sidebar_blocks → sidebar_widgets, block_type → widget_type.
-- Seed default sidebar widgets migrated from the hardcoded RightSidebar.svelte.
-- Ref: Section 59 (Sidebar Management System), ADR-066

BEGIN;

-- ── Rename table: sidebar_blocks → sidebar_widgets ─────────────────────
ALTER TABLE sidebar_blocks RENAME TO sidebar_widgets;

-- ── Rename column: block_type → widget_type ────────────────────────────
ALTER TABLE sidebar_widgets RENAME COLUMN block_type TO widget_type;

-- ── Rename index ───────────────────────────────────────────────────────
ALTER INDEX IF EXISTS idx_sidebar_blocks_sidebar RENAME TO idx_sidebar_widgets_sidebar;

-- ── Seed default sidebar: Main app, right position, all pages ──────────
-- This sidebar replaces the hardcoded widgets in RightSidebar.svelte.
-- The widgets are seeded in the same order they currently appear:
--   1. search (topbar, sort_order 0)
--   2. theme_toggle (topbar, sort_order 1)
--   3. language_toggle (topbar, sort_order 2)
--   4. user_dropdown (topbar, sort_order 3)
--   5. ad_slot sidebar_top (content, sort_order 0)
--   6. trending_tags (content, sort_order 1)
--   7. suggested_spaces (content, sort_order 2)
--   8. ad_slot sidebar_bottom (content, sort_order 3)
INSERT INTO sidebars (id, tenant_id, app, position, slug, name, page_pattern, visibility, status)
SELECT
    'a1b2c3d4-0001-4000-8000-000000000001'::uuid,
    t.id,
    'main',
    'right',
    'main-default-right',
    'Main Default Right',
    '*',
    'all',
    'active'
FROM tenants t
WHERE NOT EXISTS (
    SELECT 1 FROM sidebars
    WHERE tenant_id = t.id AND app = 'main' AND position = 'right' AND slug = 'main-default-right'
)
AND t.id = '00000000-0000-0000-0000-000000000001'::uuid;

-- Seed the default widgets for the main-default-right sidebar.
-- Only seed for the default tenant (00000000-0000-0000-0000-000000000001).
INSERT INTO sidebar_widgets (tenant_id, sidebar_id, widget_type, label, props, sort_order, visibility, status)
SELECT
    '00000000-0000-0000-0000-000000000001'::uuid,
    s.id,
    wt.widget_type,
    wt.label,
    wt.props::jsonb,
    wt.sort_order,
    'all',
    'active'
FROM sidebars s
CROSS JOIN (VALUES
    ('search',          'Search',          '{}'::jsonb, 0),
    ('theme_toggle',    'Theme Toggle',    '{}'::jsonb, 1),
    ('language_toggle', 'Language Toggle', '{}'::jsonb, 2),
    ('user_dropdown',   'User Dropdown',   '{}'::jsonb, 3),
    ('ad_slot',         'Ad Top',          '{"slot":"sidebar_top","format":"square"}'::jsonb, 4),
    ('trending_tags',   'Trending Tags',   '{"limit":8,"title":"Trending Tags"}'::jsonb, 5),
    ('suggested_spaces','Suggested Spaces','{"limit":5,"title":"Suggested Spaces"}'::jsonb, 6),
    ('ad_slot',         'Ad Bottom',       '{"slot":"sidebar_bottom","format":"banner"}'::jsonb, 7)
) AS wt(widget_type, label, props, sort_order)
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND s.app = 'main' AND s.position = 'right' AND s.slug = 'main-default-right'
  AND NOT EXISTS (
      SELECT 1 FROM sidebar_widgets sw
      WHERE sw.sidebar_id = s.id AND sw.widget_type = wt.widget_type AND sw.sort_order = wt.sort_order
  );

COMMIT;
