-- 000757_seed_onboarding_tour_steps
-- Seeds the onboarding.tour_steps system setting (Section 09) so the
-- per-surface product tour step lists are visible and editable from the
-- Control settings panel without a code deploy. Value is a JSON map of
-- surface slug to ordered data-tour target keys; frontends translate the
-- keys via their i18n dictionaries and skip steps with no visible target.

BEGIN;

INSERT INTO system_settings (key, value, is_public)
SELECT 'onboarding.tour_steps',
       '{"main":["primary-nav","home","notifications","messages","stories","forum","discover","explore","learn","jobs","achievements","bookmarks","search","space-switcher","compose","sidebar","account"],"docs":["docs-nav","docs-audience","docs-search","docs-topbar","docs-sidebar"],"learn":["primary-nav","learn-browse","my-learning","certificates","learn-bookmarks","instructor","learn-search","learn-sidebar","learn-account"]}'::jsonb,
       false
WHERE NOT EXISTS (
    SELECT 1 FROM system_settings WHERE key = 'onboarding.tour_steps'
);

COMMIT;
