-- 000375_attestation_is_japabot_indexed_generated.down.sql
-- Reverse: drop the GENERATED column and restore the plain boolean default false.

BEGIN;

ALTER TABLE attestation_pathways DROP COLUMN IF EXISTS is_japabot_indexed;
ALTER TABLE attestation_pathways ADD COLUMN is_japabot_indexed BOOLEAN NOT NULL DEFAULT false;

COMMIT;
