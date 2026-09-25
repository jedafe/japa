-- 000401_fix_digest_editions_schema.up.sql
-- Fix BUG-007: align digest_editions schema with §84 spec.
-- - Add DEFAULT 'system' to generated_by (spec: generated_by TEXT DEFAULT 'system')
-- - Update status comment to reflect the full lifecycle: draft|review|published|cancelled
-- - Backfill any NULL generated_by values to 'system'
-- Ref: japaconnect-engineering/sections/84-community-digest-widget.md

BEGIN;

-- Backfill NULL generated_by values to 'system' before adding DEFAULT.
UPDATE digest_editions SET generated_by = 'system' WHERE generated_by IS NULL;

-- Add DEFAULT 'system' to generated_by.
ALTER TABLE digest_editions
    ALTER COLUMN generated_by SET DEFAULT 'system',
    ALTER COLUMN generated_by SET NOT NULL;

-- Update the status column comment to reflect the full §84 lifecycle.
COMMENT ON COLUMN digest_editions.status IS 'draft|review|published|cancelled (§84)';

COMMIT;
