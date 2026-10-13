-- 000841_sidebar_route_family_seeds.down.sql
--
-- Remove the W18 route-family sidebars (widgets cascade via
-- sidebar_blocks_sidebar_id_fkey). The generic main-default-right fallback
-- is untouched.

DELETE FROM public.sidebars
WHERE slug IN (
    'main-forum-right',
    'main-story-right',
    'main-dashboard-right',
    'main-calendar-right',
    'main-jobs-right',
    'main-wiki-right',
    'main-universities-right',
    'main-services-right'
);
