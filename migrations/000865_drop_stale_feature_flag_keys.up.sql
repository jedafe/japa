-- 000865_drop_stale_feature_flag_keys.up.sql
-- Ref: .jeos/sections/42-feature-flags.md; same defect class as
-- 000812_drop_dead_flag_keys.
--
-- The 000735 baseline seed is a snapshot of a live DB taken while the
-- feature.{name} -> {domain}.enabled rename (archived 000404) was mid-flight:
-- it inserts BOTH key forms for 24 V2 features. The stored feature.* rows are
-- inert — every route gate and service reads the renamed key — but they
-- surface as phantom "Custom" entries in Control → Extensions and in the
-- public GET /api/v1/feature-flags map, and toggling them changes nothing.
--
-- The same UPDATE also clears three renamed-sub-toggle keys
-- (wiki.community_edits, digest.email, scam_alert.anonymous_reports) whose
-- feature_flags rows are read from system_settings, not feature_flags, and
-- test.sync.* orphans left by roadmap sync tests that ran against a shared
-- dev database.

-- Unlink any roadmap item first: deleting the row bypasses the config-change
-- hook (SyncFlagPhase) that would normally clear the link, leaving the item
-- pointing at a nonexistent flag with a frozen derived status.
UPDATE roadmap_items
SET feature_flag_key = NULL
WHERE feature_flag_key IN (
    'feature.achievement', 'feature.ama', 'feature.appointment_alert',
    'feature.arrival_checklist', 'feature.attestation_guide', 'feature.calendar',
    'feature.community_locator', 'feature.credential_checker', 'feature.cv_formatter',
    'feature.digest', 'feature.housing_guide', 'feature.japa_stage',
    'feature.language_test_hub', 'feature.mentorship', 'feature.mentorship_comparison',
    'feature.mutual_aid', 'feature.planning_group', 'feature.scam_alert',
    'feature.service_directory', 'feature.story', 'feature.tax_calculator',
    'feature.visa_routes', 'feature.visa_timeline', 'feature.wiki',
    'feature.wiki_community_edits', 'feature.scam_alert_anonymous_reports',
    'feature.digest_email',
    'wiki.community_edits', 'digest.email', 'scam_alert.anonymous_reports'
) OR feature_flag_key LIKE 'test.sync.%';

-- Drop them; the real keys are unaffected.
DELETE FROM feature_flags
WHERE key IN (
    'feature.achievement', 'feature.ama', 'feature.appointment_alert',
    'feature.arrival_checklist', 'feature.attestation_guide', 'feature.calendar',
    'feature.community_locator', 'feature.credential_checker', 'feature.cv_formatter',
    'feature.digest', 'feature.housing_guide', 'feature.japa_stage',
    'feature.language_test_hub', 'feature.mentorship', 'feature.mentorship_comparison',
    'feature.mutual_aid', 'feature.planning_group', 'feature.scam_alert',
    'feature.service_directory', 'feature.story', 'feature.tax_calculator',
    'feature.visa_routes', 'feature.visa_timeline', 'feature.wiki',
    'feature.wiki_community_edits', 'feature.scam_alert_anonymous_reports',
    'feature.digest_email',
    'wiki.community_edits', 'digest.email', 'scam_alert.anonymous_reports'
) OR key LIKE 'test.sync.%';
