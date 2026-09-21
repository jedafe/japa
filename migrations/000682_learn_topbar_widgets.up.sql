-- 000682_learn_topbar_widgets.up.sql
-- Seed the four default topbar widgets (search, theme_toggle,
-- language_toggle, user_dropdown) onto the Learn app's right sidebars.
--
-- Migration 000223 was meant to do this, but the Learn sidebars in deployed
-- environments ended up without them — the resolve payload for
-- app=learn returns only content-slot widgets, so the Learn shell renders no
-- sidebar topbar at all: no search input, no theme/language toggles, and no
-- user menu (logout/settings unreachable from the Learn surface).
--
-- The widgets are inserted at sort_order 0-3 so they render in the sticky
-- topbar above the content widgets (Section 59, ADR-066). The registry marks
-- all four types as slot='topbar' and Apps=all, so no other changes are
-- needed for /sidebars/resolve to emit them correctly.

BEGIN;

INSERT INTO sidebar_widgets (sidebar_id, widget_type, label, props, sort_order, visibility, status)
SELECT s.id, wt.widget_type, wt.label, wt.props::jsonb, wt.sort_order, 'all', 'active'
FROM sidebars s
CROSS JOIN (VALUES
    ('search',          'Search',          '{}'::jsonb, 0),
    ('theme_toggle',    'Theme Toggle',    '{}'::jsonb, 1),
    ('language_toggle', 'Language Toggle', '{}'::jsonb, 2),
    ('user_dropdown',   'User Dropdown',   '{}'::jsonb, 3)
) AS wt(widget_type, label, props, sort_order)
WHERE s.app = 'learn' AND s.position = 'right'
  AND s.slug IN ('learn-default-right', 'learn-lesson-right')
  AND NOT EXISTS (
      SELECT 1 FROM sidebar_widgets sw
      WHERE sw.sidebar_id = s.id AND sw.widget_type = wt.widget_type
  );

COMMIT;
