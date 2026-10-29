-- 000953_menu_item_feature_flags.up.sql
--
-- Backfill menu_items.feature_flag for seeded nav entries whose destination
-- routes are feature-gated (§42). Until now only dashboard_rail items carried
-- the column, so GET /menus/{slug} could not prune gated entries server-side
-- and correctness rested on the frontend routeFeatureFlags map alone.
--
-- The URL→flag map applies to every menu (main sidebar, bottom bar, footer,
-- header, user dropdown, docs header/footer, academy/learn menus) — a nav
-- link to a gated route must hide wherever it renders. The one exception is
-- '/bookmarks', restricted to the main-app menus: in academy/learn menus it
-- means course bookmarks (gated by feature.academy on that surface), not
-- feature.bookmarks.

UPDATE public.menu_items mi
SET feature_flag = m.flag
FROM (VALUES
        ('/stories',            'story.enabled'),
        ('/story',              'story.enabled'),
        ('/notifications',      'feature.notifications'),
        ('/radar',              'feature.follow'),
        ('/me/notifications',   'feature.notifications'),
        ('/messages',           'feature.dm'),
        ('/dashboard/bookmarks','feature.bookmarks'),
        ('/achievements',       'achievement.enabled'),
        ('/japa-stage',         'japa_stage.enabled'),
        ('/scam-alerts',        'scam_alert.enabled'),
        ('/mutual-aid',         'mutual_aid.enabled'),
        ('/planning-groups',    'planning_group.enabled'),
        ('/calendar',           'calendar.enabled'),
        ('/digest',             'digest.enabled'),
        ('/ama',                'forum.ama.enabled'),
        ('/services',           'service_directory.enabled'),
        ('/jobs',               'jobs.enabled'),
        ('/challenges',         'gamification.enabled'),
        ('/leaderboard',        'gamification.enabled'),
        ('/mentors',            'mentorship.enabled'),
        ('/tools/remittance',   'remittance.enabled'),
        ('/forum',              'feature.forum'),
        ('/blog',               'feature.blog'),
        ('/academy',            'feature.academy'),
        ('/learn',              'feature.academy'),
        ('/compare',            'feature.comparison'),
        ('/comparisons',        'feature.comparison'),
        ('/roadmap',            'roadmap.enabled'),
        ('/dashboard/wallet',   'feature.wallet'),
        ('/settings/wallet',    'feature.wallet'),
        ('/dashboard/affiliate','feature.affiliate_program'),
        ('/dashboard/tickets',  'feature.tickets'),
        ('/donate',             'tips.enabled'),
        ('/wiki',               'wiki.enabled'),
        ('/lists',              'list.enabled'),
        ('/list',               'list.enabled'),
        ('/video',              'video.enabled'),
        ('/studio',             'studio.enabled'),
        ('/studios',            'studio.enabled'),
        ('/deals',              'service_coupons.enabled'),
        ('/deal',               'service_coupons.enabled'),
        ('/coupons',            'service_coupons.enabled'),
        ('/series',             'feature.blog'),
        ('/visa-checklist',     'checklist.enabled'),
        ('/certificates',       'feature.academy'),
        ('/me/tips',            'tips.enabled'),
        ('/me/service-bookings','service_directory.enabled'),
        ('/me/applications',    'jobs.enabled'),
        ('/me/saved',           'calendar.enabled')
     ) AS m(url, flag)
WHERE mi.url = m.url
  AND mi.feature_flag IS NULL;

-- /bookmarks is feature.bookmarks only inside the main-app menus — in
-- academy/learn menus it is the course-bookmarks surface (feature.academy).
UPDATE public.menu_items mi
SET feature_flag = 'feature.bookmarks'
FROM public.menus mn
WHERE mi.menu_id = mn.id
  AND mi.url = '/bookmarks'
  AND mi.feature_flag IS NULL
  AND mn.slug IN ('main_sidebar', 'main_bottom_bar', 'main_footer',
                  'main_header', 'main_user_dropdown');
