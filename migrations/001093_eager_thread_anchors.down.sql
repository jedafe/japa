-- 001093 down — remove backfilled anchor rows and the system bot user.
-- thread_anchors + content_spaces cascade from the contents delete.
DELETE FROM contents
WHERE author_id = '9c7c2a1d-2c47-4e3b-9f9d-7a1b9e6a2f02'
  AND space_id = '9c7c2a1d-2c47-4e3b-9f9d-7a1b9e6a2f01';

DELETE FROM public.users WHERE id = '9c7c2a1d-2c47-4e3b-9f9d-7a1b9e6a2f02';
