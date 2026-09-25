-- 000224_sidebar_new_widget_seeds.down.sql
-- Remove the new widget types seeded by 000224.
-- Only deletes widgets with the new widget types that were added in the
-- widget registry expansion — existing widgets from earlier migrations
-- are untouched.

BEGIN;

DELETE FROM sidebar_widgets
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND widget_type IN (
    'continue_learning',
    'learning_enrollments',
    'progress_ring',
    'track_card',
    'curriculum_rail',
    'metric_card',
    'notification_centre',
    'table_of_contents',
    'search_launcher',
    'helpful_widget',
    'related_discussions',
    'leaderboard',
    'tag_cloud',
    'featured_content',
    'bookmark_folders',
    'newsletter_signup',
    'social_links',
    'cta_button',
    'upcoming_events',
    'user_card',
    'space_categories'
  );

COMMIT;
