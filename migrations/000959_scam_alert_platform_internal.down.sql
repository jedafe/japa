BEGIN;
UPDATE scam_reports SET platform_used = 'japaconnect' WHERE platform_used = 'internal';
ALTER TABLE scam_reports DROP CONSTRAINT scam_reports_platform_used_check;
ALTER TABLE scam_reports
    ADD CONSTRAINT scam_reports_platform_used_check
    CHECK (platform_used IS NULL OR platform_used IN (
        'whatsapp', 'instagram', 'linkedin', 'email', 'website', 'japaconnect', 'other'
    ));
COMMIT;
