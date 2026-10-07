-- 000802_sidebar_notification_centre.down.sql
-- Remove the seeded notification_centre widgets and restore the sort_order
-- shift the up migration applied around them.

BEGIN;

-- Reverse the step-1 shift first, while the bell rows still mark the freed
-- slot: every widget at a sort_order above the bell was bumped +1 by the up
-- migration. Sidebars with no notification_centre get a NULL subquery and
-- are skipped.
UPDATE sidebar_widgets sw
SET sort_order = sw.sort_order - 1
FROM sidebars s
WHERE sw.sidebar_id = s.id
  AND s.app = 'main' AND s.position = 'right'
  AND sw.widget_type <> 'notification_centre'
  AND sw.sort_order > (
      SELECT MIN(nc.sort_order) FROM sidebar_widgets nc
      WHERE nc.sidebar_id = s.id AND nc.widget_type = 'notification_centre'
  );

DELETE FROM sidebar_widgets sw
USING sidebars s
WHERE sw.sidebar_id = s.id
  AND sw.widget_type = 'notification_centre'
  AND s.app = 'main' AND s.position = 'right';

COMMIT;
