-- 000375_attestation_is_japabot_indexed_generated.up.sql
-- Fix BUG-011: is_japabot_indexed should be a GENERATED ALWAYS AS column
-- per §97 spec (status = 'published' AND is_active = TRUE) STORED.
-- The original migration (000337) created it as a plain boolean default false.
--
-- Ref: japaconnect-engineering/sections/97-attestation-guide.md

BEGIN;

-- Drop the plain column and recreate as a GENERATED STORED column.
-- Existing values are recomputed from status + is_active.
ALTER TABLE attestation_pathways DROP COLUMN IF EXISTS is_japabot_indexed;
ALTER TABLE attestation_pathways ADD COLUMN is_japabot_indexed BOOLEAN GENERATED ALWAYS AS (
    status = 'published' AND is_active = TRUE
) STORED;

COMMIT;
