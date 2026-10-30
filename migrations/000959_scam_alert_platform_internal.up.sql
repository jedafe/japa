-- 000959_scam_alert_platform_internal.up.sql
-- Platform classification is now binary: 'internal' = the JapaConnect
-- ecosystem itself (replaces 'japaconnect'), every other value is external.
-- On-platform entity is captured via subject_type/subject_id/subject_label.

BEGIN;

UPDATE scam_reports SET platform_used = 'internal' WHERE platform_used = 'japaconnect';

ALTER TABLE scam_reports DROP CONSTRAINT scam_reports_platform_used_check;
ALTER TABLE scam_reports
    ADD CONSTRAINT scam_reports_platform_used_check
    CHECK (platform_used IS NULL OR platform_used IN (
        'internal', 'whatsapp', 'instagram', 'linkedin', 'email', 'website', 'other'
    ));

COMMIT;
