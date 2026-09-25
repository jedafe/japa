-- 000417_scam_alert_status_enum_align.up.sql
-- Align scam_reports status + scam_type + platform_used enums to §81 spec.
--
-- §81 defines:
--   status: under_review|confirmed|debunked|duplicate  (default 'under_review')
--   scam_type: fraudulent_agent|fake_job|housing_fraud|fake_scholarship|whatsapp_telegram|other
--   platform_used: whatsapp|instagram|linkedin|email|website|other
--
-- The original migration 000304 used pending|confirmed|rejected and wrong
-- enum comments. This migration:
--   1. Rewrites existing 'pending' rows → 'under_review'
--   2. Rewrites existing 'rejected' rows → 'debunked'
--   3. Changes the column default to 'under_review'
--   4. Adds CHECK constraints for all three enum columns
--
-- Ref: japaconnect-engineering/sections/81-scam-alert-board.md
--      japaconnect-engineering/adr/ADR-089-scam-alert-board.md

BEGIN;

-- 1. Migrate existing status values to the §81 enum.
UPDATE scam_reports SET status = 'under_review' WHERE status = 'pending';
UPDATE scam_reports SET status = 'debunked'      WHERE status = 'rejected';

-- 2. Change the default to 'under_review' (§81).
ALTER TABLE scam_reports
    ALTER COLUMN status SET DEFAULT 'under_review';

-- 3. Add CHECK constraints enforcing the §81 enums.
ALTER TABLE scam_reports
    ADD CONSTRAINT scam_reports_status_check
    CHECK (status IN ('under_review', 'confirmed', 'debunked', 'duplicate'));

ALTER TABLE scam_reports
    ADD CONSTRAINT scam_reports_scam_type_check
    CHECK (scam_type IN (
        'fraudulent_agent', 'fake_job', 'housing_fraud',
        'fake_scholarship', 'whatsapp_telegram', 'other'
    ));

ALTER TABLE scam_reports
    ADD CONSTRAINT scam_reports_platform_used_check
    CHECK (platform_used IS NULL OR platform_used IN (
        'whatsapp', 'instagram', 'linkedin', 'email', 'website', 'other'
    ));

COMMIT;
