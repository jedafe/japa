-- Restore pre-PTS-001 references (entity-scoped). Only touches rows whose
-- reference ends with the row's own user_id suffix.
UPDATE public.activity_events
SET reference = left(reference, -37)
WHERE activity_key IN ('space.joined', 'follow.created')
  AND right(reference, 37) = ':' || user_id::text;
