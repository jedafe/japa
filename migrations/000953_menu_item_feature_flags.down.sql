-- 000953_menu_item_feature_flags.down.sql — revert the URL-keyed backfill.
UPDATE public.menu_items
SET feature_flag = NULL
WHERE url IN (
        '/stories','/story','/notifications','/me/notifications','/messages',
        '/radar',
        '/bookmarks','/dashboard/bookmarks','/achievements','/japa-stage',
        '/scam-alerts','/mutual-aid','/planning-groups','/calendar','/digest',
        '/ama','/services','/jobs','/challenges','/leaderboard','/mentors',
        '/tools/remittance','/forum','/blog','/academy','/learn','/compare',
        '/comparisons','/roadmap','/dashboard/wallet','/settings/wallet',
        '/dashboard/affiliate','/dashboard/tickets','/donate','/wiki','/lists',
        '/list','/video','/studio','/studios','/deals','/deal','/coupons',
        '/series','/visa-checklist','/certificates','/me/tips',
        '/me/service-bookings','/me/applications','/me/saved'
  );
