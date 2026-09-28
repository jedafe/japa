-- 000747_intel_review_log_nullable_actor.up.sql
-- intel_review_log.actor_user_id: allow NULL for system-generated audit
-- entries. Worker-side writes (dedup conflict notes, promote empty-record
-- refusals) have no human actor; a NULL actor denotes "system". Previously
-- those writes violated the NOT NULL constraint and failed the task.

BEGIN;

ALTER TABLE public.intel_review_log
    ALTER COLUMN actor_user_id DROP NOT NULL;

COMMIT;
