-- 000224_sidebar_new_widget_seeds.up.sql
-- Seed the 21 new widget types (added in the widget registry expansion) into
-- their relevant sidebars across Main, Docs, and Learn apps.
-- Ref: Section 59 (Sidebar Management System), ADR-066.
--
-- Widget placement rationale:
--   Main default (*):      community widgets — leaderboard, featured_content,
--                          upcoming_events, newsletter_signup, social_links, cta_button
--   Main user (/u/*):      user_card (profile user), related_discussions
--   Main space (/spaces/*): space_categories, recent_content
--   Main org (/org/*):     user_card (org leader), related_discussions
--   Main tag (/tags/*):    tag_cloud, featured_content
--   Main blog (/blog/*):   user_card (author), related_discussions, newsletter_signup, social_links
--   Main content (/content/*): user_card (author), related_discussions, bookmark_folders
--   Main search (/search*): trending_tags, tag_cloud
--   Docs (*):              table_of_contents, helpful_widget, social_links, cta_button
--   Learn default (*):     continue_learning, track_card, learning_enrollments, progress_ring, newsletter_signup
--   Learn lesson (/tracks/*): progress_ring, continue_learning, curriculum_rail
--
-- New widgets are appended AFTER existing widgets (sort_order continues from
-- the current max + 1 per sidebar).

BEGIN;

-- ════════════════════════════════════════════════════════════════════════
-- Main app — main-default-right (*)
-- Current max sort_order: 7. New widgets start at 8.
-- ════════════════════════════════════════════════════════════════════════
INSERT INTO sidebar_widgets (tenant_id, sidebar_id, widget_type, label, props, sort_order, visibility, status)
SELECT '00000000-0000-0000-0000-000000000001'::uuid, s.id, wt.widget_type, wt.label, wt.props::jsonb, wt.sort_order, wt.visibility, 'active'
FROM sidebars s
CROSS JOIN (VALUES
    ('leaderboard',       'Top Contributors',  '{"limit":5,"title":"Top Contributors","metric":"karma"}'::jsonb,  8, 'all'),
    ('featured_content',  'Featured',          '{"limit":3,"title":"Featured"}'::jsonb,                            9, 'all'),
    ('upcoming_events',   'Upcoming Events',   '{"limit":3,"title":"Upcoming Events"}'::jsonb,                   10, 'all'),
    ('newsletter_signup', 'Newsletter',        '{"title":"Subscribe","placeholder":"your@email.com","button_text":"Subscribe"}'::jsonb, 11, 'all'),
    ('social_links',      'Follow Us',         '{"title":"Follow Us","links":[{"platform":"twitter","url":""},{"platform":"facebook","url":""},{"platform":"instagram","url":""}]}'::jsonb, 12, 'all'),
    ('cta_button',        'Get Started',       '{"label":"Get Started","url":"/signup","variant":"primary"}'::jsonb, 13, 'guest')
) AS wt(widget_type, label, props, sort_order, visibility)
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND s.app = 'main' AND s.position = 'right' AND s.slug = 'main-default-right'
  AND NOT EXISTS (SELECT 1 FROM sidebar_widgets sw WHERE sw.sidebar_id = s.id AND sw.widget_type = wt.widget_type);

-- ════════════════════════════════════════════════════════════════════════
-- Main app — main-user-right (/u/*)
-- Current max sort_order: 6. New widgets start at 7.
-- ════════════════════════════════════════════════════════════════════════
INSERT INTO sidebar_widgets (tenant_id, sidebar_id, widget_type, label, props, sort_order, visibility, status)
SELECT '00000000-0000-0000-0000-000000000001'::uuid, s.id, wt.widget_type, wt.label, wt.props::jsonb, wt.sort_order, wt.visibility, 'active'
FROM sidebars s
CROSS JOIN (VALUES
    ('user_card',           'Profile User',     '{"user_id":"","title":"User"}'::jsonb,                             7, 'all'),
    ('related_discussions', 'Related Discussions', '{"limit":5,"title":"Discussions"}'::jsonb,                      8, 'all')
) AS wt(widget_type, label, props, sort_order, visibility)
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND s.app = 'main' AND s.position = 'right' AND s.slug = 'main-user-right'
  AND NOT EXISTS (SELECT 1 FROM sidebar_widgets sw WHERE sw.sidebar_id = s.id AND sw.widget_type = wt.widget_type);

-- ════════════════════════════════════════════════════════════════════════
-- Main app — main-space-right (/spaces/*)
-- Current max sort_order: 6. New widgets start at 7.
-- ════════════════════════════════════════════════════════════════════════
INSERT INTO sidebar_widgets (tenant_id, sidebar_id, widget_type, label, props, sort_order, visibility, status)
SELECT '00000000-0000-0000-0000-000000000001'::uuid, s.id, wt.widget_type, wt.label, wt.props::jsonb, wt.sort_order, wt.visibility, 'active'
FROM sidebars s
CROSS JOIN (VALUES
    ('space_categories', 'Browse Spaces',    '{"title":"Browse Spaces"}'::jsonb,                                 7, 'all'),
    ('recent_content',   'Recent Posts',     '{"limit":5,"title":"Recent Posts","space_id":""}'::jsonb,           8, 'all')
) AS wt(widget_type, label, props, sort_order, visibility)
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND s.app = 'main' AND s.position = 'right' AND s.slug = 'main-space-right'
  AND NOT EXISTS (SELECT 1 FROM sidebar_widgets sw WHERE sw.sidebar_id = s.id AND sw.widget_type = wt.widget_type);

-- ════════════════════════════════════════════════════════════════════════
-- Main app — main-org-right (/org/*)
-- Current max sort_order: 6. New widgets start at 7.
-- ════════════════════════════════════════════════════════════════════════
INSERT INTO sidebar_widgets (tenant_id, sidebar_id, widget_type, label, props, sort_order, visibility, status)
SELECT '00000000-0000-0000-0000-000000000001'::uuid, s.id, wt.widget_type, wt.label, wt.props::jsonb, wt.sort_order, wt.visibility, 'active'
FROM sidebars s
CROSS JOIN (VALUES
    ('user_card',           'Org Leader',    '{"user_id":"","title":"Org Leader"}'::jsonb,                        7, 'all'),
    ('related_discussions', 'Discussions',   '{"limit":5,"title":"Discussions"}'::jsonb,                          8, 'all')
) AS wt(widget_type, label, props, sort_order, visibility)
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND s.app = 'main' AND s.position = 'right' AND s.slug = 'main-org-right'
  AND NOT EXISTS (SELECT 1 FROM sidebar_widgets sw WHERE sw.sidebar_id = s.id AND sw.widget_type = wt.widget_type);

-- ════════════════════════════════════════════════════════════════════════
-- Main app — main-tag-right (/tags/*)
-- Current max sort_order: 6. New widgets start at 7.
-- ════════════════════════════════════════════════════════════════════════
INSERT INTO sidebar_widgets (tenant_id, sidebar_id, widget_type, label, props, sort_order, visibility, status)
SELECT '00000000-0000-0000-0000-000000000001'::uuid, s.id, wt.widget_type, wt.label, wt.props::jsonb, wt.sort_order, wt.visibility, 'active'
FROM sidebars s
CROSS JOIN (VALUES
    ('tag_cloud',         'Popular Tags',    '{"limit":20,"title":"Popular Tags"}'::jsonb,                       7, 'all'),
    ('featured_content',  'Featured',        '{"limit":3,"title":"Featured"}'::jsonb,                            8, 'all')
) AS wt(widget_type, label, props, sort_order, visibility)
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND s.app = 'main' AND s.position = 'right' AND s.slug = 'main-tag-right'
  AND NOT EXISTS (SELECT 1 FROM sidebar_widgets sw WHERE sw.sidebar_id = s.id AND sw.widget_type = wt.widget_type);

-- ════════════════════════════════════════════════════════════════════════
-- Main app — main-blog-right (/blog/*)
-- Current max sort_order: 6. New widgets start at 7.
-- ════════════════════════════════════════════════════════════════════════
INSERT INTO sidebar_widgets (tenant_id, sidebar_id, widget_type, label, props, sort_order, visibility, status)
SELECT '00000000-0000-0000-0000-000000000001'::uuid, s.id, wt.widget_type, wt.label, wt.props::jsonb, wt.sort_order, wt.visibility, 'active'
FROM sidebars s
CROSS JOIN (VALUES
    ('user_card',           'Author',         '{"user_id":"","title":"Author"}'::jsonb,                           7, 'all'),
    ('related_discussions', 'Discussions',    '{"limit":5,"title":"Discussions"}'::jsonb,                          8, 'all'),
    ('newsletter_signup',   'Newsletter',     '{"title":"Subscribe","placeholder":"your@email.com","button_text":"Subscribe"}'::jsonb, 9, 'all'),
    ('social_links',        'Follow Us',      '{"title":"Follow Us","links":[{"platform":"twitter","url":""},{"platform":"facebook","url":""},{"platform":"instagram","url":""}]}'::jsonb, 10, 'all')
) AS wt(widget_type, label, props, sort_order, visibility)
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND s.app = 'main' AND s.position = 'right' AND s.slug = 'main-blog-right'
  AND NOT EXISTS (SELECT 1 FROM sidebar_widgets sw WHERE sw.sidebar_id = s.id AND sw.widget_type = wt.widget_type);

-- ════════════════════════════════════════════════════════════════════════
-- Main app — main-content-right (/content/*)
-- Current max sort_order: 6. New widgets start at 7.
-- ════════════════════════════════════════════════════════════════════════
INSERT INTO sidebar_widgets (tenant_id, sidebar_id, widget_type, label, props, sort_order, visibility, status)
SELECT '00000000-0000-0000-0000-000000000001'::uuid, s.id, wt.widget_type, wt.label, wt.props::jsonb, wt.sort_order, wt.visibility, 'active'
FROM sidebars s
CROSS JOIN (VALUES
    ('user_card',           'Author',         '{"user_id":"","title":"Author"}'::jsonb,                           7, 'all'),
    ('related_discussions', 'Discussions',    '{"limit":5,"title":"Discussions"}'::jsonb,                          8, 'all'),
    ('bookmark_folders',    'Bookmarks',      '{"title":"Bookmarks"}'::jsonb,                                     9, 'authenticated')
) AS wt(widget_type, label, props, sort_order, visibility)
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND s.app = 'main' AND s.position = 'right' AND s.slug = 'main-content-right'
  AND NOT EXISTS (SELECT 1 FROM sidebar_widgets sw WHERE sw.sidebar_id = s.id AND sw.widget_type = wt.widget_type);

-- ════════════════════════════════════════════════════════════════════════
-- Main app — search-page (/search*)
-- Current max sort_order: 1. New widgets start at 2.
-- ════════════════════════════════════════════════════════════════════════
INSERT INTO sidebar_widgets (tenant_id, sidebar_id, widget_type, label, props, sort_order, visibility, status)
SELECT '00000000-0000-0000-0000-000000000001'::uuid, s.id, wt.widget_type, wt.label, wt.props::jsonb, wt.sort_order, wt.visibility, 'active'
FROM sidebars s
CROSS JOIN (VALUES
    ('trending_tags', 'Trending Tags',  '{"limit":8,"title":"Trending Tags"}'::jsonb,  2, 'all'),
    ('tag_cloud',     'Popular Tags',   '{"limit":20,"title":"Popular Tags"}'::jsonb, 3, 'all')
) AS wt(widget_type, label, props, sort_order, visibility)
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND s.app = 'main' AND s.position = 'right' AND s.slug = 'search-page'
  AND NOT EXISTS (SELECT 1 FROM sidebar_widgets sw WHERE sw.sidebar_id = s.id AND sw.widget_type = wt.widget_type);

-- ════════════════════════════════════════════════════════════════════════
-- Docs app — docs-default-right (*)
-- Current max sort_order: 5. New widgets start at 6.
-- ════════════════════════════════════════════════════════════════════════
INSERT INTO sidebar_widgets (tenant_id, sidebar_id, widget_type, label, props, sort_order, visibility, status)
SELECT '00000000-0000-0000-0000-000000000001'::uuid, s.id, wt.widget_type, wt.label, wt.props::jsonb, wt.sort_order, wt.visibility, 'active'
FROM sidebars s
CROSS JOIN (VALUES
    ('table_of_contents', 'On this page',  '{"title":"On this page"}'::jsonb,                                  6, 'all'),
    ('helpful_widget',    'Was this helpful?', '{}'::jsonb,                                                          7, 'all'),
    ('social_links',      'Follow Us',     '{"title":"Follow Us","links":[{"platform":"twitter","url":""},{"platform":"facebook","url":""},{"platform":"instagram","url":""}]}'::jsonb, 8, 'all'),
    ('cta_button',        'Try JapaConnect', '{"label":"Try JapaConnect","url":"/signup","variant":"primary"}'::jsonb, 9, 'guest')
) AS wt(widget_type, label, props, sort_order, visibility)
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND s.app = 'docs' AND s.position = 'right' AND s.slug = 'docs-default-right'
  AND NOT EXISTS (SELECT 1 FROM sidebar_widgets sw WHERE sw.sidebar_id = s.id AND sw.widget_type = wt.widget_type);

-- ════════════════════════════════════════════════════════════════════════
-- Learn app — learn-default-right (*)
-- Current max sort_order: 4. New widgets start at 5.
-- ════════════════════════════════════════════════════════════════════════
INSERT INTO sidebar_widgets (tenant_id, sidebar_id, widget_type, label, props, sort_order, visibility, status)
SELECT '00000000-0000-0000-0000-000000000001'::uuid, s.id, wt.widget_type, wt.label, wt.props::jsonb, wt.sort_order, wt.visibility, 'active'
FROM sidebars s
CROSS JOIN (VALUES
    ('continue_learning',    'Continue Learning', '{"title":"Continue Learning"}'::jsonb,                              5, 'authenticated'),
    ('track_card',           'Featured Track',    '{"track_slug":"","title":"Featured Track"}'::jsonb,                 6, 'all'),
    ('learning_enrollments', 'My Learning',       '{"limit":3,"title":"My Learning"}'::jsonb,                          7, 'authenticated'),
    ('progress_ring',        'My Progress',       '{"title":"My Progress"}'::jsonb,                                    8, 'authenticated'),
    ('newsletter_signup',    'Newsletter',        '{"title":"Subscribe","placeholder":"your@email.com","button_text":"Subscribe"}'::jsonb, 9, 'all')
) AS wt(widget_type, label, props, sort_order, visibility)
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND s.app = 'learn' AND s.position = 'right' AND s.slug = 'learn-default-right'
  AND NOT EXISTS (SELECT 1 FROM sidebar_widgets sw WHERE sw.sidebar_id = s.id AND sw.widget_type = wt.widget_type);

-- ════════════════════════════════════════════════════════════════════════
-- Learn app — learn-lesson-right (/tracks/*)
-- Current max sort_order: 4. New widgets start at 5.
-- ════════════════════════════════════════════════════════════════════════
INSERT INTO sidebar_widgets (tenant_id, sidebar_id, widget_type, label, props, sort_order, visibility, status)
SELECT '00000000-0000-0000-0000-000000000001'::uuid, s.id, wt.widget_type, wt.label, wt.props::jsonb, wt.sort_order, wt.visibility, 'active'
FROM sidebars s
CROSS JOIN (VALUES
    ('progress_ring',     'Lesson Progress',  '{"title":"My Progress"}'::jsonb,                                    5, 'authenticated'),
    ('continue_learning', 'Next Lesson',      '{"title":"Continue Learning"}'::jsonb,                              6, 'authenticated'),
    ('curriculum_rail',   'Curriculum',       '{}'::jsonb,                                                         7, 'all')
) AS wt(widget_type, label, props, sort_order, visibility)
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
  AND s.app = 'learn' AND s.position = 'right' AND s.slug = 'learn-lesson-right'
  AND NOT EXISTS (SELECT 1 FROM sidebar_widgets sw WHERE sw.sidebar_id = s.id AND sw.widget_type = wt.widget_type);

COMMIT;
