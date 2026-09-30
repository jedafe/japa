-- 000757_seed_onboarding_tour_steps (down)

BEGIN;

DELETE FROM system_settings WHERE key = 'onboarding.tour_steps';

COMMIT;
