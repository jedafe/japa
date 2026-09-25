-- Add rejected_at column to track admin-rejected media.
-- When an admin rejects media, rejected_at is set to now() and confirmed = false.
-- This allows the admin list endpoint to distinguish "rejected" from "pending"
-- (both have confirmed = false and no quarantine_reason).
ALTER TABLE media ADD COLUMN IF NOT EXISTS rejected_at timestamptz;

-- Backfill: any media that was reviewed (reviewed_by IS NOT NULL) but is neither
-- confirmed nor quarantined is effectively rejected.
UPDATE media
SET rejected_at = reviewed_at
WHERE reviewed_by IS NOT NULL
  AND confirmed = false
  AND (quarantine_reason IS NULL OR quarantine_reason = '')
  AND rejected_at IS NULL;
