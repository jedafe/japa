-- 000223_sidebar_default_widgets.up.sql
-- Add the default topbar widgets (search, theme_toggle, language_toggle,
-- user_dropdown) to all route-specific Main app sidebars that are missing them.
-- Ref: Section 59 (Sidebar Management System), ADR-066.
--
-- main-default-right already has these (seeded in 000220). The route-specific
-- sidebars (user, space, org, tag, blog, content) only got content-slot widgets
-- in 000221/000222. This migration:
--   1. Shifts existing content widgets up by 4 (to make room for topbar widgets
--      at sort_order 0-3).
--   2. Inserts the 4 topbar widgets at sort_order 0-3.

BEGIN;

-- ── Step 1: Shift existing content widgets up by 4 ──────────────────────
UPDATE sidebar_widgets sw
SET sort_order = sw.sort_order + 4
FROM sidebars s
WHERE sw.sidebar_id = s.id
  AND s.tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND s.app = 'main'
  AND s.position = 'right'
  AND s.slug IN (
    'main-user-right', 'main-space-right', 'main-org-right', 'main-tag-right',
    'main-blog-right', 'main-content-right'
  );

-- ── Step 2: Insert the 4 default topbar widgets at sort_order 0-3 ───────
INSERT INTO sidebar_widgets (tenant_id, sidebar_id, widget_type, label, props, sort_order, visibility, status)
SELECT '00000000-0000-0000-0000-000000000001'::uuid, s.id, wt.widget_type, wt.label, wt.props::jsonb, wt.sort_order, 'all', 'active'
FROM sidebars s
CROSS JOIN (VALUES
    ('search',          'Search',          '{}'::jsonb, 0),
    ('theme_toggle',    'Theme Toggle',    '{}'::jsonb, 1),
    ('language_toggle', 'Language Toggle', '{}'::jsonb, 2),
    ('user_dropdown',   'User Dropdown',   '{}'::jsonb, 3)
) AS wt(widget_type, label, props, sort_order)
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND s.app = 'main' AND s.position = 'right'
  AND s.slug IN (
    'main-user-right', 'main-space-right', 'main-org-right', 'main-tag-right',
    'main-blog-right', 'main-content-right'
  )
  AND NOT EXISTS (
      SELECT 1 FROM sidebar_widgets sw
      WHERE sw.sidebar_id = s.id AND sw.widget_type = wt.widget_type
  );

-- ── Step 3: Add default topbar widgets to Docs and Learn sidebars ───────
-- Docs and Learn sidebars also need the topbar widgets (search, theme_toggle,
-- language_toggle, user_dropdown). Shift existing content widgets up by 4 first.
UPDATE sidebar_widgets sw
SET sort_order = sw.sort_order + 4
FROM sidebars s
WHERE sw.sidebar_id = s.id
  AND s.tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND s.app IN ('docs', 'learn')
  AND s.position = 'right'
  AND s.slug IN ('docs-default-right', 'learn-default-right', 'learn-lesson-right');

INSERT INTO sidebar_widgets (tenant_id, sidebar_id, widget_type, label, props, sort_order, visibility, status)
SELECT '00000000-0000-0000-0000-000000000001'::uuid, s.id, wt.widget_type, wt.label, wt.props::jsonb, wt.sort_order, 'all', 'active'
FROM sidebars s
CROSS JOIN (VALUES
    ('search',          'Search',          '{}'::jsonb, 0),
    ('theme_toggle',    'Theme Toggle',    '{}'::jsonb, 1),
    ('language_toggle', 'Language Toggle', '{}'::jsonb, 2),
    ('user_dropdown',   'User Dropdown',   '{}'::jsonb, 3)
) AS wt(widget_type, label, props, sort_order)
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND s.app IN ('docs', 'learn')
  AND s.position = 'right'
  AND s.slug IN ('docs-default-right', 'learn-default-right', 'learn-lesson-right')
  AND NOT EXISTS (
      SELECT 1 FROM sidebar_widgets sw
      WHERE sw.sidebar_id = s.id AND sw.widget_type = wt.widget_type
  );

COMMIT;
