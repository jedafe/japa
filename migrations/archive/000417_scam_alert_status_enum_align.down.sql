-- 000417_scam_alert_status_enum_align.down.sql
-- Reverse the §81 enum alignment. Removes the CHECK constraints and
-- restores the original (pre-§81) default + values so the schema matches
-- migration 000304 again. Existing 'under_review'/'debunked' rows are
-- rewritten back to 'pending'/'rejected'.

BEGIN;

ALTER TABLE scam_reports DROP CONSTRAINT IF EXISTS scam_reports_platform_used_check;
ALTER TABLE scam_reports DROP CONSTRAINT IF EXISTS scam_reports_scam_type_check;
ALTER TABLE scam_reports DROP CONSTRAINT IF EXISTS scam_reports_status_check;

ALTER TABLE scam_reports
    ALTER COLUMN status SET DEFAULT 'pending';

UPDATE scam_reports SET status = 'pending'  WHERE status = 'under_review';
UPDATE scam_reports SET status = 'rejected' WHERE status = 'debunked';
UPDATE scam_reports SET status = 'rejected' WHERE status = 'duplicate';

COMMIT;
