-- Rename V2 feature flag keys from feature.{name} to {domain}.enabled /
-- {domain}.{sub} per conventions/feature-flag-naming.md and V2 section specs.
-- V1 keys (feature.blog, feature.forum, etc.) are unchanged — they are locked
-- in Section 42 and use the feature.{name} format.
-- Only V2 keys (§§79–102) are renamed to match the convention.

UPDATE feature_flags SET key = 'mentorship.enabled'          WHERE key = 'feature.mentorship';
UPDATE feature_flags SET key = 'mentorship.comparison'       WHERE key = 'feature.mentorship_comparison';
UPDATE feature_flags SET key = 'story.enabled'               WHERE key = 'feature.story';
UPDATE feature_flags SET key = 'wiki.enabled'                WHERE key = 'feature.wiki';
UPDATE feature_flags SET key = 'wiki.community_edits'        WHERE key = 'feature.wiki_community_edits';
UPDATE feature_flags SET key = 'scam_alert.enabled'          WHERE key = 'feature.scam_alert';
UPDATE feature_flags SET key = 'scam_alert.anonymous_reports' WHERE key = 'feature.scam_alert_anonymous_reports';
UPDATE feature_flags SET key = 'planning_group.enabled'      WHERE key = 'feature.planning_group';
UPDATE feature_flags SET key = 'visa_timeline.enabled'       WHERE key = 'feature.visa_timeline';
UPDATE feature_flags SET key = 'digest.enabled'              WHERE key = 'feature.digest';
UPDATE feature_flags SET key = 'digest.email'                WHERE key = 'feature.digest_email';
UPDATE feature_flags SET key = 'mutual_aid.enabled'          WHERE key = 'feature.mutual_aid';
UPDATE feature_flags SET key = 'calendar.enabled'            WHERE key = 'feature.calendar';
UPDATE feature_flags SET key = 'japa_stage.enabled'          WHERE key = 'feature.japa_stage';
UPDATE feature_flags SET key = 'forum.ama.enabled'           WHERE key = 'feature.ama';
UPDATE feature_flags SET key = 'achievement.enabled'         WHERE key = 'feature.achievement';
UPDATE feature_flags SET key = 'service_directory.enabled'   WHERE key = 'feature.service_directory';
UPDATE feature_flags SET key = 'visa_routes.enabled'         WHERE key = 'feature.visa_routes';
UPDATE feature_flags SET key = 'appointment_alert.enabled'   WHERE key = 'feature.appointment_alert';
UPDATE feature_flags SET key = 'arrival_checklist.enabled'   WHERE key = 'feature.arrival_checklist';
UPDATE feature_flags SET key = 'credential_checker.enabled'  WHERE key = 'feature.credential_checker';
UPDATE feature_flags SET key = 'cv_formatter.enabled'        WHERE key = 'feature.cv_formatter';
UPDATE feature_flags SET key = 'housing_guide.enabled'       WHERE key = 'feature.housing_guide';
UPDATE feature_flags SET key = 'attestation_guide.enabled'   WHERE key = 'feature.attestation_guide';
UPDATE feature_flags SET key = 'language_test_hub.enabled'   WHERE key = 'feature.language_test_hub';
UPDATE feature_flags SET key = 'tax_calculator.enabled'      WHERE key = 'feature.tax_calculator';
UPDATE feature_flags SET key = 'community_locator.enabled'   WHERE key = 'feature.community_locator';
