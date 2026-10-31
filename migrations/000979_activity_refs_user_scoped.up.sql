-- PTS-001 follow-up: rewrite historical space.joined / follow.created
-- activity_event references into the new actor-scoped format
-- ("{entity}:{user_id}") so leave/rejoin and unfollow/refollow by a user
-- who earned under the old global reference still dedupes correctly.
-- Without this, a user who earned under the old-format reference could
-- earn once more under the new-format reference. Rows already carrying
-- the :{user_id} suffix are skipped (idempotent).
UPDATE public.activity_events
SET reference = reference || ':' || user_id::text
WHERE activity_key = 'space.joined'
  AND right(reference, 37) <> ':' || user_id::text;

UPDATE public.activity_events
SET reference = reference || ':' || user_id::text
WHERE activity_key = 'follow.created'
  AND right(reference, 37) <> ':' || user_id::text;
