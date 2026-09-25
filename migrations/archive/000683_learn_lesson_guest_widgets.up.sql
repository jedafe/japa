-- 000683_learn_lesson_guest_widgets.up.sql
-- learn-lesson-right (/tracks/*) previously carried only an ad_slot plus
-- authenticated-only widgets (progress_ring, continue_learning) and the
-- no-op curriculum_rail. For guests — and for signed-in users when no ad
-- campaign is served — the sidebar's content area rendered completely
-- empty. Seed guest-visible widgets so track detail and lesson pages always
-- have useful sidebar content (Section 59, ADR-066).

BEGIN;

INSERT INTO sidebar_widgets (sidebar_id, widget_type, label, props, sort_order, visibility, status)
SELECT s.id, wt.widget_type, wt.label, wt.props::jsonb, wt.sort_order, wt.visibility, 'active'
FROM sidebars s
CROSS JOIN (VALUES
    ('track_card',        'Featured Track', '{"track_slug":"","title":"Featured Track"}'::jsonb,                                          12, 'all'),
    ('newsletter_signup', 'Newsletter',     '{"title":"Subscribe","placeholder":"your@email.com","button_text":"Subscribe"}'::jsonb,      13, 'all')
) AS wt(widget_type, label, props, sort_order, visibility)
WHERE s.app = 'learn' AND s.position = 'right'
  AND s.slug = 'learn-lesson-right'
  AND NOT EXISTS (
      SELECT 1 FROM sidebar_widgets sw
      WHERE sw.sidebar_id = s.id AND sw.widget_type = wt.widget_type
  );

COMMIT;
