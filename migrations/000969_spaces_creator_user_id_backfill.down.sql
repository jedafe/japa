-- Reverse the backfill: clear creator_user_id where it equals the space's
-- current owner (i.e. the value came from the backfill, not an explicit
-- write). Safe because creator_user_id was never populated before 000968.
UPDATE spaces s
SET creator_user_id = NULL
FROM space_members sm
WHERE sm.space_id = s.id
  AND sm.role = 'owner'
  AND sm.status = 'approved'
  AND s.creator_user_id = sm.user_id;
