-- 000802_sidebar_notification_centre.up.sql
-- Seed the notification_centre topbar widget into every Main right sidebar
-- that lacks it. The widget type is registered in the sidebar widget type
-- registry (internal/sidebars/widget_types.go, slot=topbar, apps=main) but
-- was never seeded into the route sidebars — only main-default-right has it.
-- Placement: immediately before user_dropdown (bell then avatar); when a
-- sidebar has no user_dropdown the bell goes to the head of the widget list —
-- the registry slot still renders it in the sticky top bar either way.
-- visibility='authenticated': guests have no notifications.
-- Ref: Section 59 (Sidebar Management System).

BEGIN;

-- Step 1: shift widgets at/below the user_dropdown position down by one to
-- free a slot — only in sidebars that have a user_dropdown and no
-- notification_centre yet. Sidebars without a user_dropdown get a NULL
-- subquery and are skipped (no shift needed).
UPDATE sidebar_widgets sw
SET sort_order = sw.sort_order + 1
FROM sidebars s
WHERE sw.sidebar_id = s.id
  AND s.app = 'main' AND s.position = 'right'
  AND NOT EXISTS (
      SELECT 1 FROM sidebar_widgets nc
      WHERE nc.sidebar_id = s.id AND nc.widget_type = 'notification_centre'
  )
  AND sw.sort_order >= (
      SELECT MIN(ud.sort_order) FROM sidebar_widgets ud
      WHERE ud.sidebar_id = s.id AND ud.widget_type = 'user_dropdown'
  );

-- Step 2: insert the bell into the freed slot (post-shift MIN-1 lands exactly
-- where user_dropdown used to sit) or at sort_order 0 when the sidebar has
-- no user_dropdown.
INSERT INTO sidebar_widgets (sidebar_id, widget_type, label, props, sort_order, visibility, status)
SELECT
    s.id,
    'notification_centre',
    'Notifications',
    '{}'::jsonb,
    COALESCE((
        SELECT MIN(ud.sort_order) - 1 FROM sidebar_widgets ud
        WHERE ud.sidebar_id = s.id AND ud.widget_type = 'user_dropdown'
    ), 0),
    'authenticated',
    'active'
FROM sidebars s
WHERE s.app = 'main' AND s.position = 'right'
  AND NOT EXISTS (
      SELECT 1 FROM sidebar_widgets nc
      WHERE nc.sidebar_id = s.id AND nc.widget_type = 'notification_centre'
  );

COMMIT;
