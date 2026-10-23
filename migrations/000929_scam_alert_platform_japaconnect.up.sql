-- 000929_scam_alert_platform_japaconnect.up.sql
-- Add 'japaconnect' to the scam_reports.platform_used enum — reporters can
-- flag scams that happened on the JapaConnect platform itself (§81).
--
-- Ref: japaconnect-engineering/sections/81-scam-alert-board.md

BEGIN;

ALTER TABLE scam_reports DROP CONSTRAINT scam_reports_platform_used_check;
ALTER TABLE scam_reports
    ADD CONSTRAINT scam_reports_platform_used_check
    CHECK (platform_used IS NULL OR platform_used IN (
        'whatsapp', 'instagram', 'linkedin', 'email', 'website', 'japaconnect', 'other'
    ));

COMMIT;
