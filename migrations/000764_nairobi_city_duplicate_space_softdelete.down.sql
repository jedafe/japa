UPDATE public.spaces
SET status = 'active', deleted_at = NULL
WHERE slug = 'ke-nairobi-city' AND deleted_at IS NOT NULL;
