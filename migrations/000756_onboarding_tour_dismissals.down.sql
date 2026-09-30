-- 000756_onboarding_tour_dismissals (down)

BEGIN;

ALTER TABLE onboarding_state
    DROP COLUMN IF EXISTS tour_dismissals;

COMMIT;
