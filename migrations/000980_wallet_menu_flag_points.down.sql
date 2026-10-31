UPDATE public.menu_items
SET feature_flag = 'feature.wallet'
WHERE feature_flag = 'feature.points'
  AND url IN ('/dashboard/wallet', '/settings/wallet');
