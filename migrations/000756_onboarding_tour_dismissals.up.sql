-- 000756_onboarding_tour_dismissals
-- Adds a per-surface dismissal map to onboarding_state so the product tour
-- (Section 09) can be dismissed independently on each app surface — main,
-- docs, learn — instead of sharing a single boolean. Keys are surface slugs
-- ("main", "docs", "learn"); the legacy tour_dismissed boolean continues to
-- represent the main-app tour for backward compatibility.

BEGIN;

ALTER TABLE onboarding_state
    ADD COLUMN tour_dismissals jsonb DEFAULT '{}'::jsonb NOT NULL;

COMMIT;
