-- 000960_scam_alert_platform_ref.up.sql
-- External platforms get a contact-reference field (social URL, WhatsApp/
-- Telegram number, website URL, email) captured per the §81 flow; kept
-- moderation-only like subject fields (redacted publicly).

BEGIN;

ALTER TABLE scam_reports DROP CONSTRAINT scam_reports_platform_used_check;
ALTER TABLE scam_reports
    ADD CONSTRAINT scam_reports_platform_used_check
    CHECK (platform_used IS NULL OR platform_used IN (
        'internal', 'whatsapp', 'telegram', 'facebook', 'instagram',
        'linkedin', 'email', 'website', 'other'
    ));

ALTER TABLE scam_reports ADD COLUMN IF NOT EXISTS platform_ref text;

COMMIT;
