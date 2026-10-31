-- PTS-010 follow-up: the member wallet surface is the points surface —
-- gated by feature.points (engagement), not feature.wallet (commerce).
UPDATE public.menu_items
SET feature_flag = 'feature.points'
WHERE feature_flag = 'feature.wallet'
  AND url IN ('/dashboard/wallet', '/settings/wallet');
