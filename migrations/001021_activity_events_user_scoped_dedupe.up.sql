-- 001021_activity_events_user_scoped_dedupe (PTS-015)
--
-- activity_events dedupe was global on (activity_key, reference) — every
-- multi-actor reference (challenge.completed, mentorship.connected,
-- job.saved, service.saved, and the entire TrackActivityUsage catalogue)
-- single-winner'd: only the first user to perform a shared action recorded
-- the event, so badges/challenges keyed on those activities were dead for
-- everyone else.
--
-- Section 28 scopes the idempotency key to
-- "per-tenant identifier + user_id + activity_key + reference", so the
-- constraint widens to include user_id. Existing rows keep their prior
-- (key, ref) uniqueness; adding user_id can only relax the constraint,
-- so no dedupe/backfill is required.
DROP INDEX IF EXISTS public.activity_events_activity_key_reference_key;

CREATE UNIQUE INDEX activity_events_user_key_reference_key
    ON public.activity_events USING btree (user_id, activity_key, reference);
