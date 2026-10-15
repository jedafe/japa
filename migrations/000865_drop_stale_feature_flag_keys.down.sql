-- 000865_drop_stale_feature_flag_keys.down.sql
-- The deleted rows were inert (no code path reads them); restore them as
-- disabled rather than guessing their prior enabled state — safer than
-- silently enabling a feature that was off. test.sync.* orphans are
-- intentionally not restored.
INSERT INTO feature_flags (key, enabled, updated_at)
VALUES ('feature.achievement', false, now()),
       ('feature.ama', false, now()),
       ('feature.appointment_alert', false, now()),
       ('feature.arrival_checklist', false, now()),
       ('feature.attestation_guide', false, now()),
       ('feature.calendar', false, now()),
       ('feature.community_locator', false, now()),
       ('feature.credential_checker', false, now()),
       ('feature.cv_formatter', false, now()),
       ('feature.digest', false, now()),
       ('feature.housing_guide', false, now()),
       ('feature.japa_stage', false, now()),
       ('feature.language_test_hub', false, now()),
       ('feature.mentorship', false, now()),
       ('feature.mentorship_comparison', false, now()),
       ('feature.mutual_aid', false, now()),
       ('feature.planning_group', false, now()),
       ('feature.scam_alert', false, now()),
       ('feature.service_directory', false, now()),
       ('feature.story', false, now()),
       ('feature.tax_calculator', false, now()),
       ('feature.visa_routes', false, now()),
       ('feature.visa_timeline', false, now()),
       ('feature.wiki', false, now())
ON CONFLICT (key) DO NOTHING;
