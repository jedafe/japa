-- Down migration for 000048_media_quarantine.sql
-- Removes the quarantine columns and index from the media table.
DROP INDEX IF EXISTS idx_media_quarantined;
ALTER TABLE media DROP COLUMN IF EXISTS reviewed_at;
ALTER TABLE media DROP COLUMN IF EXISTS reviewed_by;
ALTER TABLE media DROP COLUMN IF EXISTS quarantined_at;
ALTER TABLE media DROP COLUMN IF EXISTS quarantine_reason;
