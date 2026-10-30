BEGIN;
ALTER TABLE scam_reports DROP COLUMN IF EXISTS platform_ref;
ALTER TABLE scam_reports DROP CONSTRAINT scam_reports_platform_used_check;
ALTER TABLE scam_reports
    ADD CONSTRAINT scam_reports_platform_used_check
    CHECK (platform_used IS NULL OR platform_used IN (
        'internal', 'whatsapp', 'instagram', 'linkedin', 'email', 'website', 'other'
    ));
COMMIT;
