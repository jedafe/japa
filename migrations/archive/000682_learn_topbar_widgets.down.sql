-- 000682_learn_topbar_widgets.down.sql
-- Remove the seeded topbar widgets from the Learn right sidebars.

BEGIN;

DELETE FROM sidebar_widgets sw
USING sidebars s
WHERE sw.sidebar_id = s.id
  AND s.app = 'learn' AND s.position = 'right'
  AND s.slug IN ('learn-default-right', 'learn-lesson-right')
  AND sw.widget_type IN ('search', 'theme_toggle', 'language_toggle', 'user_dropdown')
  AND sw.sort_order IN (0, 1, 2, 3);

COMMIT;
