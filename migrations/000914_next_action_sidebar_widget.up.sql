-- 000914_next_action_sidebar_widget.up.sql — §122: seed the next_action
-- widget into every main right sidebar that lacks it, positioned after
-- suggested_content (or suggested_spaces) where present.
-- Ref: japaconnect-engineering/sections/122-next-action-suggestions.md.

BEGIN;

-- Shift widgets below the anchor slot down by one to free a position.
UPDATE sidebar_widgets sw
SET sort_order = sw.sort_order + 1
FROM sidebars s
WHERE sw.sidebar_id = s.id
  AND s.app = 'main' AND s.position = 'right'
  AND NOT EXISTS (
      SELECT 1 FROM sidebar_widgets na
      WHERE na.sidebar_id = s.id AND na.widget_type = 'next_action'
  )
  AND sw.sort_order > (
      SELECT COALESCE(MAX(a.sort_order), 0) FROM sidebar_widgets a
      WHERE a.sidebar_id = s.id AND a.widget_type IN ('suggested_content', 'suggested_spaces')
  );

INSERT INTO sidebar_widgets (sidebar_id, widget_type, label, props, sort_order, visibility, status)
SELECT s.id, 'next_action', 'Your next step',
       '{"limit":1,"title":"Your next step"}',
       COALESCE((SELECT MAX(a.sort_order) FROM sidebar_widgets a
                 WHERE a.sidebar_id = s.id
                   AND a.widget_type IN ('suggested_content', 'suggested_spaces')), 0) + 1,
       'all', 'active'
FROM sidebars s
WHERE s.app = 'main' AND s.position = 'right'
  AND NOT EXISTS (
      SELECT 1 FROM sidebar_widgets na
      WHERE na.sidebar_id = s.id AND na.widget_type = 'next_action'
  );

COMMIT;
