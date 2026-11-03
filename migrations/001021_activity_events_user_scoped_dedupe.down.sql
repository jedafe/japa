-- Revert: restore the global (activity_key, reference) dedupe index.
-- WARNING: rows recorded for multiple users under the same (key, ref)
-- while the widened index was live will violate this constraint — dedupe
-- them manually before rolling back.
DROP INDEX IF EXISTS public.activity_events_user_key_reference_key;

CREATE UNIQUE INDEX activity_events_activity_key_reference_key
    ON public.activity_events USING btree (activity_key, reference);
