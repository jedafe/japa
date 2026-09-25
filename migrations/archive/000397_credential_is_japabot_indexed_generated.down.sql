-- 000397_credential_is_japabot_indexed_generated.down.sql

BEGIN;

ALTER TABLE credential_pathways DROP COLUMN IF EXISTS is_japabot_indexed;
ALTER TABLE credential_pathways ADD COLUMN is_japabot_indexed BOOLEAN NOT NULL DEFAULT false;

COMMIT;
