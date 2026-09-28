-- 000747_intel_review_log_nullable_actor.down.sql
-- Fails if any rows carry a NULL actor_user_id — delete or reattribute
-- them before rolling back.

BEGIN;

ALTER TABLE public.intel_review_log
    ALTER COLUMN actor_user_id SET NOT NULL;

COMMIT;
