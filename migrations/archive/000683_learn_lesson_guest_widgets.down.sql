-- 000683_learn_lesson_guest_widgets.down.sql
-- Remove the guest-visible widgets seeded onto learn-lesson-right.

BEGIN;

DELETE FROM sidebar_widgets sw
USING sidebars s
WHERE sw.sidebar_id = s.id
  AND s.app = 'learn' AND s.position = 'right'
  AND s.slug = 'learn-lesson-right'
  AND sw.widget_type IN ('track_card', 'newsletter_signup')
  AND sw.sort_order IN (12, 13);

COMMIT;
