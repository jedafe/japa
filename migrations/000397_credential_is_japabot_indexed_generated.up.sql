-- 000397_credential_is_japabot_indexed_generated.up.sql
-- Fix BUG-001: is_japabot_indexed should be a GENERATED ALWAYS AS column
-- per §94 spec (status = 'published' AND is_active = TRUE) STORED.
-- The original migration (000331) created it as a plain boolean default false.
--
-- Ref: japaconnect-engineering/sections/94-professional-credential-checker.md
--      japaconnect-engineering/adr/ADR-102-credential-checker.md

BEGIN;

-- Drop the plain column and recreate as a GENERATED STORED column.
-- Existing values are recomputed from status + is_active.
ALTER TABLE credential_pathways DROP COLUMN IF EXISTS is_japabot_indexed;
ALTER TABLE credential_pathways ADD COLUMN is_japabot_indexed BOOLEAN GENERATED ALWAYS AS (
    status = 'published' AND is_active = TRUE
) STORED;

COMMIT;
